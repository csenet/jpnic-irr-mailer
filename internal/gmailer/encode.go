package gmailer

import "encoding/base64"

// base64URL は Gmail API が要求する URL-safe な base64 に変換する。
func base64URL(b []byte) string {
	return base64.URLEncoding.EncodeToString(b)
}
