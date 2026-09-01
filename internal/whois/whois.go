// Package whois は JPIRR の whois サーバから登録済みオブジェクトを取得する。
package whois

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/csenet/jpnic-irr-mailer/internal/rpsl"
)

// DefaultServer は JPIRR の whois サーバ。
const DefaultServer = "jpirr.nic.ad.jp:43"

// Client は 1 クエリごとに接続を張る素朴な whois クライアント。
type Client struct {
	Server  string
	Timeout time.Duration
}

// New は既定値を埋めたクライアントを返す。
func New(server string) *Client {
	if server == "" {
		server = DefaultServer
	}
	if !strings.Contains(server, ":") {
		server += ":43"
	}
	return &Client{Server: server, Timeout: 30 * time.Second}
}

// Query は生のクエリ文字列を送り、応答本文をそのまま返す。
func (c *Client) Query(ctx context.Context, query string) (string, error) {
	d := net.Dialer{Timeout: c.Timeout}
	conn, err := d.DialContext(ctx, "tcp", c.Server)
	if err != nil {
		return "", fmt.Errorf("whois %s への接続に失敗: %w", c.Server, err)
	}
	defer conn.Close()

	deadline := time.Now().Add(c.Timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)

	if _, err := io.WriteString(conn, query+"\r\n"); err != nil {
		return "", fmt.Errorf("クエリ送信に失敗: %w", err)
	}
	body, err := io.ReadAll(conn)
	if err != nil {
		return "", fmt.Errorf("応答の読み取りに失敗: %w", err)
	}
	return string(body), nil
}

// ByMaintainer は mnt-by の逆引きで maintainer 配下のオブジェクトを取得する。
// -s JPIRR でミラー分を除き、JPIRR 自身の登録内容だけに絞る。
func (c *Client) ByMaintainer(ctx context.Context, mntner string) ([]rpsl.Object, error) {
	mntner = strings.TrimSpace(mntner)
	if mntner == "" {
		return nil, fmt.Errorf("maintainer 名が空です")
	}
	body, err := c.Query(ctx, "-s JPIRR -i mnt-by "+mntner)
	if err != nil {
		return nil, err
	}
	if err := checkError(body); err != nil {
		return nil, err
	}
	return rpsl.Parse(body), nil
}

// Lookup は主キーを指定して単一オブジェクトを引く (例: 192.0.2.0/24)。
func (c *Client) Lookup(ctx context.Context, key string) ([]rpsl.Object, error) {
	body, err := c.Query(ctx, "-s JPIRR -x "+strings.TrimSpace(key))
	if err != nil {
		return nil, err
	}
	if err := checkError(body); err != nil {
		return nil, err
	}
	return rpsl.Parse(body), nil
}

// checkError は whois サーバが返すエラー行を Go の error に変換する。
// 「該当なし」は空の結果として扱い、エラーにはしない。
func checkError(body string) error {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "%ERROR") {
			return fmt.Errorf("whois サーバがエラーを返しました: %s", line)
		}
	}
	return nil
}
