package gmailer

import (
	"strings"
	"testing"
)

// JPIRR は plain text しか受け付けないので、組み立てたメールがそれを満たすか確かめる。
func TestBuildPlainText(t *testing.T) {
	raw, err := Message{
		From:    "noc@example.jp",
		To:      "auto-dbm@nic.ad.jp",
		Subject: "JPIRR object submission (update 1)",
		Body:    "password:   x\nroute:      192.0.2.0/24\n",
	}.Build()
	if err != nil {
		t.Fatalf("Build が失敗: %v", err)
	}
	s := string(raw)
	for _, want := range []string{
		"To: auto-dbm@nic.ad.jp\r\n",
		"From: noc@example.jp\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n",
		"\r\n\r\npassword:   x\r\nroute:      192.0.2.0/24\r\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("%q が含まれていません:\n%s", want, s)
		}
	}
	if strings.Contains(s, "text/html") {
		t.Errorf("HTML パートが混入しています:\n%s", s)
	}
}

func TestBuildRejectsBadTo(t *testing.T) {
	if _, err := (Message{To: "not-an-address"}).Build(); err == nil {
		t.Fatal("不正な宛先が通ってしまいました")
	}
}
