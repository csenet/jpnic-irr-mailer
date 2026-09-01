// Package gmailer は Gmail API (OAuth2) を使った plain text メール送信を担う。
package gmailer

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// scopes は送信と、画面に出す自分のアドレス取得だけに絞る。
// メールの閲覧権限は要求しない。
var scopes = []string{
	gmail.GmailSendScope,
	"https://www.googleapis.com/auth/userinfo.email",
}

// Client は OAuth 設定とトークンの置き場所を保持する。
type Client struct {
	oauth     *oauth2.Config
	tokenPath string
}

// New は credentials.json (GCP の OAuth クライアント) を読み込む。
// redirectURL にはこのツール自身のコールバック URL を渡す。
func New(dir, redirectURL string) (*Client, error) {
	credPath := filepath.Join(dir, "credentials.json")
	b, err := os.ReadFile(credPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("OAuth クライアント情報がありません: %s に credentials.json を置いてください", credPath)
		}
		return nil, err
	}
	conf, err := google.ConfigFromJSON(b, scopes...)
	if err != nil {
		return nil, fmt.Errorf("credentials.json の解析に失敗: %w", err)
	}
	conf.RedirectURL = redirectURL
	return &Client{oauth: conf, tokenPath: filepath.Join(dir, "token.json")}, nil
}

// AuthCodeURL は同意画面の URL を組み立てる。
// refresh token を確実に受け取るため offline + consent を指定する。
func (c *Client) AuthCodeURL(state string) string {
	return c.oauth.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"))
}

// Exchange は認可コードをトークンに交換して保存する。
func (c *Client) Exchange(ctx context.Context, code string) error {
	tok, err := c.oauth.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("トークンの取得に失敗: %w", err)
	}
	return c.saveToken(tok)
}

// Token は保存済みトークンを読む。未認証なら nil を返す。
func (c *Client) Token() *oauth2.Token {
	b, err := os.ReadFile(c.tokenPath)
	if err != nil {
		return nil
	}
	var tok oauth2.Token
	if err := json.Unmarshal(b, &tok); err != nil {
		return nil
	}
	return &tok
}

// Authorized は使えるトークンがあるかを返す。
func (c *Client) Authorized() bool {
	tok := c.Token()
	return tok != nil && (tok.Valid() || tok.RefreshToken != "")
}

// Logout は保存済みトークンを削除する。
func (c *Client) Logout() error {
	err := os.Remove(c.tokenPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (c *Client) saveToken(tok *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(c.tokenPath), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.tokenPath, append(b, '\n'), 0o600)
}

// service は自動リフレッシュ付きの Gmail クライアントを返す。
// リフレッシュで更新されたトークンは保存し直す。
func (c *Client) service(ctx context.Context) (*gmail.Service, error) {
	tok := c.Token()
	if tok == nil {
		return nil, fmt.Errorf("Gmail が未認証です。先に Google ログインを済ませてください")
	}
	src := c.oauth.TokenSource(ctx, tok)
	if fresh, err := src.Token(); err == nil && fresh.AccessToken != tok.AccessToken {
		_ = c.saveToken(fresh)
	}
	return gmail.NewService(ctx, option.WithTokenSource(src))
}

// Address は認証中の Google アカウントのアドレスを返す。
// Gmail の GetProfile は gmail.send だけでは呼べないため、
// userinfo.email スコープで読める userinfo エンドポイントを使う。
func (c *Client) Address(ctx context.Context) (string, error) {
	tok := c.Token()
	if tok == nil {
		return "", fmt.Errorf("Gmail が未認証です")
	}
	src := c.oauth.TokenSource(ctx, tok)
	if fresh, err := src.Token(); err == nil && fresh.AccessToken != tok.AccessToken {
		_ = c.saveToken(fresh)
	}
	httpClient := oauth2.NewClient(ctx, src)
	resp, err := httpClient.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return "", fmt.Errorf("アカウント情報の取得に失敗: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("アカウント情報の取得に失敗: HTTP %d", resp.StatusCode)
	}
	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("アカウント情報の解析に失敗: %w", err)
	}
	return info.Email, nil
}

// Message は送信する 1 通のメール。JPIRR 宛なので本文は常に plain text。
type Message struct {
	From    string
	To      string
	Subject string
	Body    string
}

// Build は RFC 5322 形式のメールを組み立てる。
// JPIRR の自動処理は HTML メールを受け付けないため、text/plain 単体で送る。
func (m Message) Build() ([]byte, error) {
	if _, err := mail.ParseAddress(m.To); err != nil {
		return nil, fmt.Errorf("宛先アドレスが不正です (%s): %w", m.To, err)
	}
	// 本文は RPSL なので改行を CRLF に揃えておく。
	body := strings.ReplaceAll(m.Body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	if !strings.HasSuffix(body, "\r\n") {
		body += "\r\n"
	}

	var h strings.Builder
	if m.From != "" {
		fmt.Fprintf(&h, "From: %s\r\n", m.From)
	}
	fmt.Fprintf(&h, "To: %s\r\n", m.To)
	fmt.Fprintf(&h, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", m.Subject))
	h.WriteString("MIME-Version: 1.0\r\n")
	h.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	h.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	h.WriteString("\r\n")
	return []byte(h.String() + body), nil
}

// Send はメールを送り、Gmail のメッセージ ID を返す。
func (c *Client) Send(ctx context.Context, m Message) (string, error) {
	raw, err := m.Build()
	if err != nil {
		return "", err
	}
	svc, err := c.service(ctx)
	if err != nil {
		return "", err
	}
	sent, err := svc.Users.Messages.Send("me", &gmail.Message{
		Raw: base64URL(raw),
	}).Do()
	if err != nil {
		return "", fmt.Errorf("メール送信に失敗: %w", err)
	}
	return sent.Id, nil
}
