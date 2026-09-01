// Package submit は画面で作った変更セットを auto-dbm 宛メールの本文に組み立てる。
package submit

import (
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/csenet/jpnic-irr-mailer/internal/descrypt"
	"github.com/csenet/jpnic-irr-mailer/internal/rpsl"
)

// Action はオブジェクトに対する操作。
type Action string

const (
	Create Action = "create"
	Update Action = "update"
	Delete Action = "delete"
)

// Change は 1 オブジェクト分の変更。
type Change struct {
	Action Action      `json:"action"`
	Object rpsl.Object `json:"object"`
}

// Request は 1 通のメールにまとめる変更セット。
type Request struct {
	Password string   `json:"password"` // JPIRR の maintainer パスワード (保存しない)
	From     string   `json:"from"`     // changed:/delete: に書くメールアドレス
	Changes  []Change `json:"changes"`
	Date     string   `json:"date"` // YYYYMMDD。空なら今日
}

// Result は組み立て結果。Body はパスワードを含む実際の送信本文、
// Preview は password 行を伏せた画面表示用。
type Result struct {
	Body     string   `json:"body"`
	Preview  string   `json:"preview"`
	Subject  string   `json:"subject"`
	Warnings []string `json:"warnings"`
}

var (
	asRe     = regexp.MustCompile(`^(?i)AS\d+$`)
	mntnerRe = regexp.MustCompile(`^[A-Za-z0-9_.\-]+$`)
)

// hiddenCryptPW は JPIRR の whois が CRYPT-PW のハッシュを伏せるときに使う文字列。
const hiddenCryptPW = "HIDDENCRYPTPW"

// Build は変更セットを検証してメール本文を作る。
// 検証に落ちた場合はエラーを返し、本文は作らない。
func Build(req Request) (*Result, error) {
	if strings.TrimSpace(req.Password) == "" {
		return nil, fmt.Errorf("JPIRR のパスワードが未入力です")
	}
	if _, err := parseAddr(req.From); err != nil {
		return nil, err
	}
	if len(req.Changes) == 0 {
		return nil, fmt.Errorf("送信する変更がありません")
	}
	date := req.Date
	if date == "" {
		date = time.Now().Format("20060102")
	}
	if len(date) != 8 {
		return nil, fmt.Errorf("日付は YYYYMMDD 形式で指定してください: %q", date)
	}

	var warnings []string
	var blocks, previews []string

	for i, ch := range req.Changes {
		obj, warn, err := normalize(ch, req.From, date, req.Password)
		if err != nil {
			return nil, fmt.Errorf("%d 件目 (%s %s): %w", i+1, ch.Object.Class(), ch.Object.Key(), err)
		}
		warnings = append(warnings, warn...)

		// JPNIC の記入例にならい、オブジェクトごとに password 行を先頭へ置く。
		withPw := obj.Clone()
		withPw.Attrs = append([]rpsl.Attr{{Name: "password", Value: req.Password}}, withPw.Attrs...)
		blocks = append(blocks, withPw.String())

		masked := obj.Clone()
		masked.Attrs = append([]rpsl.Attr{{Name: "password", Value: "********"}}, masked.Attrs...)
		previews = append(previews, masked.String())
	}

	return &Result{
		Body:     strings.Join(blocks, "\n"),
		Preview:  strings.Join(previews, "\n"),
		Subject:  subject(req.Changes),
		Warnings: warnings,
	}, nil
}

// subject は件名を作る。auto-dbm は件名を見ないが、送信控えを探しやすくしておく。
func subject(changes []Change) string {
	n := map[Action]int{}
	for _, c := range changes {
		n[c.Action]++
	}
	var parts []string
	for _, a := range []Action{Create, Update, Delete} {
		if n[a] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", a, n[a]))
		}
	}
	return "JPIRR object submission (" + strings.Join(parts, ", ") + ")"
}

// normalize は 1 オブジェクトを検証し、changed:/delete:/source: を整えて返す。
func normalize(ch Change, from, date, password string) (rpsl.Object, []string, error) {
	obj := ch.Object.Clone()

	// 空値の属性は送る前に落とす (画面の未入力欄がそのまま残るのを防ぐ)。
	kept := obj.Attrs[:0]
	for _, a := range obj.Attrs {
		if strings.TrimSpace(a.Value) != "" && strings.TrimSpace(a.Name) != "" {
			kept = append(kept, a)
		}
	}
	obj.Attrs = kept

	if len(obj.Attrs) == 0 {
		return obj, nil, fmt.Errorf("属性が 1 つもありません")
	}
	// password は本文組み立て時に付けるので、画面由来のものは捨てる。
	obj.Remove("password")

	class := obj.Class()
	warnings, err := validate(obj, class, ch.Action)
	if err != nil {
		return obj, nil, err
	}

	// whois は mntner の CRYPT-PW ハッシュを HIDDENCRYPTPW に伏せて返す。
	// そのまま送ると auth が壊れて締め出されるので、入力されたパスワードから作り直して差し替える。
	if class == "mntner" {
		replaced, err := rehashHiddenAuth(&obj, password)
		if err != nil {
			return obj, nil, err
		}
		if replaced > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"%s: auth の伏字 (%s) %d 件を、入力されたパスワードの CRYPT-PW ハッシュに置き換えました",
				obj.Key(), hiddenCryptPW, replaced))
		}
	}

	switch ch.Action {
	case Create, Update:
		obj.Remove("delete")
		obj.Set("changed", from+" "+date)
	case Delete:
		// changed: は登録時のものを残したまま delete: を足すのが JPNIC の手順。
		if obj.Get("changed") == "" {
			obj.Set("changed", from+" "+date)
		}
		obj.Remove("delete")
		obj.InsertBefore("source", "delete", from+" "+date)
	default:
		return obj, nil, fmt.Errorf("不明な操作です: %q", ch.Action)
	}

	obj.Set("source", "JPIRR")
	// source は末尾に置くのが慣例なので、最後へ移し替える。
	moveLast(&obj, "source")

	return obj, warnings, nil
}

// requireAll は列挙した属性がすべて非空であることを確かめる。
func requireAll(obj rpsl.Object, names ...string) error {
	for _, n := range names {
		if strings.TrimSpace(obj.Get(n)) == "" {
			return fmt.Errorf("%s は必須です", n)
		}
	}
	return nil
}

// rehashHiddenAuth は伏字になっている CRYPT-PW の auth を、password から作ったハッシュで置き換える。
// 置き換えた件数を返す。PGPKEY など CRYPT-PW 以外の auth は触らない。
func rehashHiddenAuth(o *rpsl.Object, password string) (int, error) {
	n := 0
	for i := range o.Attrs {
		a := &o.Attrs[i]
		if !strings.EqualFold(a.Name, "auth") || !strings.Contains(a.Value, hiddenCryptPW) {
			continue
		}
		salt, err := descrypt.NewSalt()
		if err != nil {
			return n, fmt.Errorf("salt の生成に失敗: %w", err)
		}
		hash, err := descrypt.Crypt(password, salt)
		if err != nil {
			return n, fmt.Errorf("CRYPT-PW の生成に失敗: %w", err)
		}
		a.Value = "CRYPT-PW " + hash
		n++
	}
	return n, nil
}

// moveLast は指定属性を末尾へ移動する。
// 元スライスを in-place で詰めると移動元の値が上書きされるので、必ず新しいスライスに組み直す。
func moveLast(o *rpsl.Object, name string) {
	var target *rpsl.Attr
	kept := make([]rpsl.Attr, 0, len(o.Attrs))
	for _, a := range o.Attrs {
		if strings.EqualFold(a.Name, name) && target == nil {
			t := a
			target = &t
			continue
		}
		kept = append(kept, a)
	}
	if target != nil {
		kept = append(kept, *target)
	}
	o.Attrs = kept
}

// validate はクラスごとの必須項目と値の形式を確かめる。
func validate(obj rpsl.Object, class string, action Action) ([]string, error) {
	var warnings []string

	key := strings.TrimSpace(obj.Key())
	if key == "" {
		return nil, fmt.Errorf("%s の値が空です", class)
	}

	switch class {
	case "route", "route6":
		p, err := netip.ParsePrefix(key)
		if err != nil {
			return nil, fmt.Errorf("プレフィックスとして解釈できません: %q", key)
		}
		if class == "route" && !p.Addr().Is4() {
			return nil, fmt.Errorf("route には IPv4 を書いてください (IPv6 は route6): %q", key)
		}
		if class == "route6" && p.Addr().Is4() {
			return nil, fmt.Errorf("route6 には IPv6 を書いてください (IPv4 は route): %q", key)
		}
		if p.Addr() != p.Masked().Addr() {
			return nil, fmt.Errorf("プレフィックスがネットワークアドレスになっていません: %q (正しくは %s)", key, p.Masked())
		}
		origin := obj.Get("origin")
		if origin == "" {
			return nil, fmt.Errorf("origin は必須です")
		}
		if !asRe.MatchString(origin) {
			return nil, fmt.Errorf("origin は AS 番号で書いてください: %q", origin)
		}
		if n, err := strconv.ParseUint(strings.TrimPrefix(strings.ToUpper(origin), "AS"), 10, 32); err == nil && n == 0 {
			warnings = append(warnings, "origin が AS0 になっています")
		}
		if obj.Get("descr") == "" {
			return nil, fmt.Errorf("descr は必須です")
		}
	case "aut-num":
		if !asRe.MatchString(key) {
			return nil, fmt.Errorf("aut-num は AS 番号で書いてください: %q", key)
		}
		if err := requireAll(obj, "as-name", "descr", "admin-c", "tech-c"); err != nil {
			return nil, err
		}
	case "as-set":
		if !strings.HasPrefix(strings.ToUpper(key), "AS-") {
			return nil, fmt.Errorf("as-set 名は AS- で始めてください: %q", key)
		}
		if err := requireAll(obj, "descr", "members"); err != nil {
			return nil, err
		}
	case "person", "role":
		// JPIRR にはハンドルの自動採番が無い。nic-hdl は JPNIC 割り当て済みのものが必要。
		if err := requireAll(obj, "nic-hdl", "address", "phone"); err != nil {
			return nil, err
		}
		if strings.HasPrefix(strings.ToUpper(obj.Get("nic-hdl")), "AUTO-") {
			return nil, fmt.Errorf("nic-hdl に AUTO- は使えません。JPIRR はハンドルを自動採番しないので、JPNIC から割り当て済みのハンドルを書いてください")
		}
		if obj.Get("e-mail") == "" {
			warnings = append(warnings, key+": e-mail がありません (JPNIC の記入例には含まれています)")
		}
	case "mntner":
		// 新規登録だけは auto-dbm ではなく irr-admin@nic.ad.jp への申請。
		// 既存 mntner の変更・削除は password 付きで auto-dbm に送れる。
		if action == Create {
			return nil, fmt.Errorf("mntner の新規登録はこのツールからは送信できません (irr-admin@nic.ad.jp への申請が必要です)")
		}
		auths := obj.GetAll("auth")
		if len(auths) == 0 {
			return nil, fmt.Errorf("mntner には auth が必須です")
		}
	}

	mntBy := obj.Get("mnt-by")
	if mntBy == "" {
		return nil, fmt.Errorf("mnt-by は必須です")
	}
	if !mntnerRe.MatchString(mntBy) {
		return nil, fmt.Errorf("mnt-by の形式が不正です: %q", mntBy)
	}

	// JPIRR は ASCII 前提。全角や日本語が混ざると自動処理で弾かれる。
	for _, a := range obj.Attrs {
		if i := strings.IndexFunc(a.Value, func(r rune) bool { return r > 127 }); i >= 0 {
			warnings = append(warnings,
				fmt.Sprintf("%s: 非 ASCII 文字が含まれています (JPIRR は ASCII のみ受け付けます)", a.Name))
		}
	}
	return warnings, nil
}

// parseAddr は changed: に書くアドレスをざっくり検証する。
func parseAddr(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("送信元メールアドレスが未設定です")
	}
	if !strings.Contains(s, "@") || strings.ContainsAny(s, " \t") {
		return "", fmt.Errorf("送信元メールアドレスが不正です: %q", s)
	}
	return s, nil
}
