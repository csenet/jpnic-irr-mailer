package submit

import (
	"strings"
	"testing"

	"github.com/csenet/jpnic-irr-mailer/internal/descrypt"
	"github.com/csenet/jpnic-irr-mailer/internal/rpsl"
)

// whois の実応答を模した入力。継続行 (X-keiro) を含む。
const sample = `route:      192.0.2.0/24
descr:      Example Inc.
            X-keiro : alert@example.jp
origin:     AS64496
member-of:  RS-EXAMPLE
notify:     noc@example.jp
mnt-by:     MAINT-AS64496
changed:    noc@example.jp 20240101
source:     JPIRR
`

func parseOne(t *testing.T, s string) rpsl.Object {
	t.Helper()
	objs := rpsl.Parse(s)
	if len(objs) != 1 {
		t.Fatalf("オブジェクト数が %d 件、期待は 1 件", len(objs))
	}
	return objs[0]
}

// パースして書き戻すと元の整形に一致すること。
func TestRoundTrip(t *testing.T) {
	got := parseOne(t, sample).String()
	if got != sample {
		t.Errorf("round trip がずれました\n--- got ---\n%s\n--- want ---\n%s", got, sample)
	}
}

func TestBuildUpdate(t *testing.T) {
	obj := parseOne(t, sample)
	res, err := Build(Request{
		Password: "s3cret",
		From:     "noc@example.jp",
		Date:     "20260901",
		Changes:  []Change{{Action: Update, Object: obj}},
	})
	if err != nil {
		t.Fatalf("Build が失敗: %v", err)
	}
	if !strings.HasPrefix(res.Body, "password:   s3cret\n") {
		t.Errorf("password 行が先頭にありません:\n%s", res.Body)
	}
	if !strings.Contains(res.Body, "changed:    noc@example.jp 20260901\n") {
		t.Errorf("changed が今回の日付に更新されていません:\n%s", res.Body)
	}
	if !strings.HasSuffix(strings.TrimRight(res.Body, "\n"), "source:     JPIRR") {
		t.Errorf("source が末尾にありません:\n%s", res.Body)
	}
	if strings.Contains(res.Body, "delete:") {
		t.Errorf("更新なのに delete が入っています:\n%s", res.Body)
	}
	if strings.Contains(res.Preview, "s3cret") {
		t.Errorf("プレビューにパスワードが露出しています:\n%s", res.Preview)
	}
}

func TestBuildDelete(t *testing.T) {
	obj := parseOne(t, sample)
	res, err := Build(Request{
		Password: "s3cret",
		From:     "noc@example.jp",
		Date:     "20260901",
		Changes:  []Change{{Action: Delete, Object: obj}},
	})
	if err != nil {
		t.Fatalf("Build が失敗: %v", err)
	}
	// 削除では登録時の changed を残したまま delete を足し、source の直前に置く。
	if !strings.Contains(res.Body, "changed:    noc@example.jp 20240101\n") {
		t.Errorf("元の changed が失われています:\n%s", res.Body)
	}
	want := "delete:     noc@example.jp 20260901\nsource:     JPIRR\n"
	if !strings.HasSuffix(res.Body, want) {
		t.Errorf("delete の位置が想定と違います:\n%s", res.Body)
	}
}

func TestValidation(t *testing.T) {
	base := parseOne(t, sample)

	cases := []struct {
		name    string
		mutate  func(o *rpsl.Object)
		wantErr string
	}{
		{"ホストビットが立っている", func(o *rpsl.Object) { o.Attrs[0].Value = "192.0.2.1/24" }, "ネットワークアドレス"},
		{"route に IPv6", func(o *rpsl.Object) { o.Attrs[0].Value = "2001:db8::/32" }, "IPv4 を書いて"},
		{"origin が不正", func(o *rpsl.Object) { o.Set("origin", "64496") }, "AS 番号"},
		{"origin なし", func(o *rpsl.Object) { o.Remove("origin") }, "origin は必須"},
		{"descr なし", func(o *rpsl.Object) { o.Remove("descr") }, "descr は必須"},
		{"mnt-by なし", func(o *rpsl.Object) { o.Remove("mnt-by") }, "mnt-by は必須"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := base.Clone()
			tc.mutate(&obj)
			_, err := Build(Request{
				Password: "x", From: "noc@example.jp", Date: "20260901",
				Changes: []Change{{Action: Update, Object: obj}},
			})
			if err == nil {
				t.Fatalf("エラーになるはずが通りました")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("エラー内容が想定と違います: %v", err)
			}
		})
	}
}

// route6 のホストビット判定と、IPv6 の正常系。
func TestRoute6(t *testing.T) {
	obj := parseOne(t, strings.Replace(
		strings.Replace(sample, "route:      192.0.2.0/24", "route6:     2001:db8::/32", 1),
		"member-of:  RS-EXAMPLE\n", "", 1))
	if _, err := Build(Request{
		Password: "x", From: "noc@example.jp", Date: "20260901",
		Changes: []Change{{Action: Create, Object: obj}},
	}); err != nil {
		t.Fatalf("route6 の正常系が失敗: %v", err)
	}
}

const mntnerSample = `mntner:     MAINT-AS64496
descr:      Example maintainer
admin-c:    AA000JP
tech-c:     BB000JP
upd-to:     noc@example.jp
mnt-nfy:    noc@example.jp
auth:       CRYPT-PW lIUkAMPHFC2kE
mnt-by:     MAINT-AS64496
changed:    noc@example.jp 20240101
source:     JPIRR
`

// mntner の新規登録は irr-admin 宛の申請なので拒否する。
func TestMntnerCreateRejected(t *testing.T) {
	obj := parseOne(t, mntnerSample)
	_, err := Build(Request{
		Password: "x", From: "noc@example.jp",
		Changes: []Change{{Action: Create, Object: obj}},
	})
	if err == nil || !strings.Contains(err.Error(), "irr-admin") {
		t.Fatalf("mntner の新規登録が拒否されていません: %v", err)
	}
}

// 既存 mntner の更新 (再登録含む) は password 付きで auto-dbm に送れる。
func TestMntnerUpdateAllowed(t *testing.T) {
	obj := parseOne(t, mntnerSample)
	res, err := Build(Request{
		Password: "x", From: "noc@example.jp", Date: "20260901",
		Changes: []Change{{Action: Update, Object: obj}},
	})
	if err != nil {
		t.Fatalf("mntner の更新が通りません: %v", err)
	}
	if !strings.Contains(res.Body, "auth:       CRYPT-PW lIUkAMPHFC2kE\n") {
		t.Errorf("auth がそのまま残っていません:\n%s", res.Body)
	}
}

// whois が返す伏字の auth は、入力パスワードから作った CRYPT-PW ハッシュに差し替わること。
func TestMntnerHiddenAuthRehashed(t *testing.T) {
	obj := parseOne(t, strings.Replace(mntnerSample,
		"auth:       CRYPT-PW lIUkAMPHFC2kE",
		"auth:       CRYPT-PW HIDDENCRYPTPW\nauth:       PGPKEY-4A928EC5", 1))
	res, err := Build(Request{
		Password: "abcdefg", From: "noc@example.jp", Date: "20260901",
		Changes: []Change{{Action: Update, Object: obj}},
	})
	if err != nil {
		t.Fatalf("Build が失敗: %v", err)
	}
	if strings.Contains(res.Body, "HIDDENCRYPTPW") {
		t.Fatalf("伏字が残っています:\n%s", res.Body)
	}
	// CRYPT-PW の行を取り出し、入力パスワードで検証できるハッシュになっているか確かめる。
	var hash string
	for _, line := range strings.Split(res.Body, "\n") {
		if strings.HasPrefix(line, "auth:") && strings.Contains(line, "CRYPT-PW ") {
			hash = strings.TrimSpace(strings.SplitN(line, "CRYPT-PW ", 2)[1])
		}
	}
	if len(hash) != 13 {
		t.Fatalf("CRYPT-PW ハッシュが 13 文字ではありません: %q", hash)
	}
	got, _ := descrypt.Crypt("abcdefg", hash[:2])
	if got != hash {
		t.Errorf("ハッシュが入力パスワードと一致しません: %s", hash)
	}
	if !strings.Contains(res.Body, "auth:       PGPKEY-4A928EC5\n") {
		t.Errorf("PGPKEY の auth が触られています:\n%s", res.Body)
	}
	if len(res.Warnings) == 0 || !strings.Contains(res.Warnings[0], "置き換えました") {
		t.Errorf("置き換えの警告が出ていません: %v", res.Warnings)
	}
}

// 複数オブジェクトは空行区切りで 1 通にまとまること。
func TestMultipleObjects(t *testing.T) {
	obj := parseOne(t, sample)
	other := obj.Clone()
	other.Attrs[0].Value = "198.51.100.0/24"

	res, err := Build(Request{
		Password: "x", From: "noc@example.jp", Date: "20260901",
		Changes: []Change{
			{Action: Update, Object: obj},
			{Action: Delete, Object: other},
		},
	})
	if err != nil {
		t.Fatalf("Build が失敗: %v", err)
	}
	if n := strings.Count(res.Body, "password:"); n != 2 {
		t.Errorf("password 行が %d 個、期待は 2 個", n)
	}
	if !strings.Contains(res.Body, "\n\npassword:") {
		t.Errorf("オブジェクトが空行で区切られていません:\n%s", res.Body)
	}
	if res.Subject != "JPIRR object submission (update 1, delete 1)" {
		t.Errorf("件名が想定と違います: %s", res.Subject)
	}
}

// 非 ASCII は拒否ではなく警告として返す。
func TestNonASCIIWarning(t *testing.T) {
	obj := parseOne(t, sample)
	obj.Set("descr", "株式会社サンプル")
	res, err := Build(Request{
		Password: "x", From: "noc@example.jp", Date: "20260901",
		Changes: []Change{{Action: Update, Object: obj}},
	})
	if err != nil {
		t.Fatalf("Build が失敗: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Errorf("非 ASCII の警告が出ていません")
	}
}

// changed: が無く source: が末尾でないオブジェクトでも、changed が 1 行・source が末尾になること。
// (moveLast の in-place 詰め替えで source が消える回帰を防ぐ)
func TestSourceMovedLastWithoutCorruption(t *testing.T) {
	obj := parseOne(t, "route:      192.0.2.0/24\ndescr:      Example\norigin:     AS64496\nsource:     JPIRR\nmnt-by:     MAINT-AS64496\n")
	res, err := Build(Request{
		Password: "x", From: "noc@example.jp", Date: "20260901",
		Changes: []Change{{Action: Update, Object: obj}},
	})
	if err != nil {
		t.Fatalf("Build が失敗: %v", err)
	}
	if n := strings.Count(res.Body, "changed:"); n != 1 {
		t.Errorf("changed が %d 行あります:\n%s", n, res.Body)
	}
	if n := strings.Count(res.Body, "source:"); n != 1 {
		t.Errorf("source が %d 行あります:\n%s", n, res.Body)
	}
	if !strings.HasSuffix(res.Body, "source:     JPIRR\n") {
		t.Errorf("source が末尾にありません:\n%s", res.Body)
	}
}

const personSample = `person:     Keiro Taro
nic-hdl:    TK36JP
address:    JAPAN Internet Routing Registry Inc.
            2-3-4 Uchi-kanda
phone:      +81-3-1234-5678
e-mail:     taro@example.jp
mnt-by:     MAINT-AS64496
source:     JPIRR
`

func TestPersonCreate(t *testing.T) {
	obj := parseOne(t, personSample)
	res, err := Build(Request{
		Password: "x", From: "taro@example.jp", Date: "20260901",
		Changes: []Change{{Action: Create, Object: obj}},
	})
	if err != nil {
		t.Fatalf("person の登録が通りません: %v", err)
	}
	if !strings.Contains(res.Body, "nic-hdl:    TK36JP\n") {
		t.Errorf("nic-hdl が残っていません:\n%s", res.Body)
	}
}

func TestPersonRequiresHandle(t *testing.T) {
	for _, tc := range []struct{ name, hdl, want string }{
		{"nic-hdl なし", "", "nic-hdl は必須"},
		{"AUTO 採番", "AUTO-1", "自動採番しない"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := parseOne(t, personSample)
			if tc.hdl == "" {
				obj.Remove("nic-hdl")
			} else {
				obj.Set("nic-hdl", tc.hdl)
			}
			_, err := Build(Request{Password: "x", From: "a@b.jp",
				Changes: []Change{{Action: Create, Object: obj}}})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期待したエラーになりません: %v", err)
			}
		})
	}
}

func TestAsSetAndAutNum(t *testing.T) {
	asSet := parseOne(t, "as-set:     AS-EXAMPLE\ndescr:      Example\nmembers:    AS64496, AS64500\nmnt-by:     MAINT-AS64496\nsource:     JPIRR\n")
	autNum := parseOne(t, "aut-num:    AS64496\nas-name:    EXAMPLE\ndescr:      Example\nadmin-c:    AA000JP\ntech-c:     BB000JP\nmnt-by:     MAINT-AS64496\nsource:     JPIRR\n")
	if _, err := Build(Request{Password: "x", From: "a@b.jp", Date: "20260901",
		Changes: []Change{{Action: Create, Object: asSet}, {Action: Create, Object: autNum}}}); err != nil {
		t.Fatalf("as-set / aut-num の正常系が失敗: %v", err)
	}
	bad := asSet.Clone()
	bad.Attrs[0].Value = "EXAMPLE"
	if _, err := Build(Request{Password: "x", From: "a@b.jp",
		Changes: []Change{{Action: Create, Object: bad}}}); err == nil {
		t.Errorf("AS- で始まらない as-set が通っています")
	}
}
