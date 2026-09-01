package descrypt

import (
	"crypto/des"
	"encoding/binary"
	"testing"
)

// salt 無し (E テーブルそのまま) の 1 回暗号化が標準 DES と一致すること。
func TestPlainDESMatchesStdlib(t *testing.T) {
	key := []byte{0x13, 0x34, 0x57, 0x79, 0x9B, 0xBC, 0xDF, 0xF1}
	pt := []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF}
	c, _ := des.NewCipher(key)
	want := make([]byte, 8)
	c.Encrypt(want, pt)

	ks := keySchedule(binary.BigEndian.Uint64(key))
	e := eTable
	got := encrypt(binary.BigEndian.Uint64(pt), &ks, &e)
	if got != binary.BigEndian.Uint64(want) {
		t.Fatalf("DES 不一致: got %016x want %x", got, want)
	}
}
