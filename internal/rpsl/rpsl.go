// Package rpsl は JPIRR が用いる RPSL 形式のオブジェクトを読み書きする。
package rpsl

import (
	"bufio"
	"strings"
)

// Attr は RPSL の 1 属性。継続行は Value 内に "\n" 区切りで保持する。
type Attr struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Object は属性の並び。RPSL では属性の重複も順序も意味を持つのでスライスで持つ。
type Object struct {
	Attrs []Attr `json:"attrs"`
}

// alignCol は JPNIC の記入例に合わせた属性値の開始カラム。
const alignCol = 12

// Parse は whois 応答やメール本文から複数のオブジェクトを取り出す。
// 空行が区切り、"%" と "#" で始まる行はコメントとして捨てる。
func Parse(text string) []Object {
	var objs []Object
	var cur Object

	flush := func() {
		if len(cur.Attrs) > 0 {
			objs = append(objs, cur)
			cur = Object{}
		}
	}

	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r \t")
		switch {
		case strings.TrimSpace(line) == "":
			flush()
		case strings.HasPrefix(line, "%"), strings.HasPrefix(line, "#"):
			// whois サーバのバナーや注意書き
		case line[0] == ' ' || line[0] == '\t' || line[0] == '+':
			// 継続行。"+" 単独は空行の継続を表す。
			if len(cur.Attrs) == 0 {
				continue
			}
			cont := strings.TrimPrefix(line, "+")
			last := &cur.Attrs[len(cur.Attrs)-1]
			last.Value += "\n" + strings.TrimSpace(cont)
		default:
			name, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			cur.Attrs = append(cur.Attrs, Attr{
				Name:  strings.TrimSpace(name),
				Value: strings.TrimSpace(value),
			})
		}
	}
	flush()
	return objs
}

// Get は最初に見つかった属性値を返す。
func (o Object) Get(name string) string {
	for _, a := range o.Attrs {
		if strings.EqualFold(a.Name, name) {
			return a.Value
		}
	}
	return ""
}

// GetAll は同名の属性値をすべて返す (auth のように重複する属性向け)。
func (o Object) GetAll(name string) []string {
	var out []string
	for _, a := range o.Attrs {
		if strings.EqualFold(a.Name, name) {
			out = append(out, a.Value)
		}
	}
	return out
}

// Set は既存の属性を上書きし、無ければ末尾に追加する。
func (o *Object) Set(name, value string) {
	for i := range o.Attrs {
		if strings.EqualFold(o.Attrs[i].Name, name) {
			o.Attrs[i].Value = value
			return
		}
	}
	o.Attrs = append(o.Attrs, Attr{Name: name, Value: value})
}

// Remove は指定名の属性をすべて取り除く。
func (o *Object) Remove(name string) {
	kept := o.Attrs[:0]
	for _, a := range o.Attrs {
		if !strings.EqualFold(a.Name, name) {
			kept = append(kept, a)
		}
	}
	o.Attrs = kept
}

// InsertBefore は before の直前に属性を挿入する。before が無ければ末尾に足す。
func (o *Object) InsertBefore(before, name, value string) {
	for i, a := range o.Attrs {
		if strings.EqualFold(a.Name, before) {
			o.Attrs = append(o.Attrs, Attr{})
			copy(o.Attrs[i+1:], o.Attrs[i:])
			o.Attrs[i] = Attr{Name: name, Value: value}
			return
		}
	}
	o.Attrs = append(o.Attrs, Attr{Name: name, Value: value})
}

// Class は先頭属性の名前、つまりオブジェクトのクラス名 (route, mntner ...) を返す。
func (o Object) Class() string {
	if len(o.Attrs) == 0 {
		return ""
	}
	return strings.ToLower(o.Attrs[0].Name)
}

// Key は先頭属性の値、つまり主キー (192.0.2.0/24 など) を返す。
func (o Object) Key() string {
	if len(o.Attrs) == 0 {
		return ""
	}
	return o.Attrs[0].Value
}

// String は JPNIC の記入例と同じ桁揃えで整形する。
func (o Object) String() string {
	var b strings.Builder
	for _, a := range o.Attrs {
		label := a.Name + ":"
		pad := alignCol - len(label)
		if pad < 1 {
			pad = 1
		}
		indent := strings.Repeat(" ", alignCol)
		for i, line := range strings.Split(a.Value, "\n") {
			if i == 0 {
				b.WriteString(label + strings.Repeat(" ", pad) + line + "\n")
			} else {
				b.WriteString(indent + line + "\n")
			}
		}
	}
	return b.String()
}

// Clone は属性を複製した独立したオブジェクトを返す。
func (o Object) Clone() Object {
	attrs := make([]Attr, len(o.Attrs))
	copy(attrs, o.Attrs)
	return Object{Attrs: attrs}
}
