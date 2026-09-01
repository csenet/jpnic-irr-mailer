package descrypt

import (
	"os/exec"
	"strings"
	"testing"
)

// 正解データは Perl の crypt() (libc crypt(3)) から取る。
func perlCrypt(t *testing.T, pw, salt string) string {
	t.Helper()
	out, err := exec.Command("perl", "-e", `print crypt($ARGV[0], $ARGV[1])`, pw, salt).Output()
	if err != nil {
		t.Skipf("perl が使えないのでスキップ: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestCryptMatchesLibc(t *testing.T) {
	cases := []struct{ pw, salt string }{
		{"abcdefg", "lI"},
		{"zyxwvuts", "pf"},
		{"password", "ab"},
		{"", ".."},
		{"a", "zz"},
		{"longer-than-eight", "Aa"},
		{"P@ss w0rd!", "/9"},
	}
	for _, c := range cases {
		want := perlCrypt(t, c.pw, c.salt)
		got, err := Crypt(c.pw, c.salt)
		if err != nil {
			t.Fatalf("Crypt(%q,%q): %v", c.pw, c.salt, err)
		}
		if got != want {
			t.Errorf("Crypt(%q,%q) = %s, want %s", c.pw, c.salt, got, want)
		}
	}
}

func TestNewSalt(t *testing.T) {
	s, err := NewSalt()
	if err != nil || len(s) != 2 {
		t.Fatalf("NewSalt: %q, %v", s, err)
	}
	if _, err := Crypt("x", s); err != nil {
		t.Fatalf("生成した salt が使えません: %v", err)
	}
}
