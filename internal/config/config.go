// Package config はツールの設定と OAuth トークンをユーザのホーム配下に永続化する。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config は送信内容の既定値。
type Config struct {
	Mntner        string `json:"mntner"`        // 例: MAINT-AS64496
	From          string `json:"from"`          // 送信元。Gmail の認証アカウント
	ChangedBy     string `json:"changedBy"`     // changed:/delete: に書くアドレス。空なら From と同じ
	To            string `json:"to"`            // 既定は auto-dbm@nic.ad.jp
	WhoisServer   string `json:"whoisServer"`   // 既定は jpirr.nic.ad.jp:43
	Notify        string `json:"notify"`        // 新規オブジェクトの notify 既定値
	DefaultOrigin string `json:"defaultOrigin"` // 新規 route の origin 既定値 (例: AS64496)
	// JPIRR の maintainer パスワード。空なら送信のたびに画面で入力する。
	// 保存する場合も config.json は 0600 で書くので、他ユーザーからは読めない。
	Password string `json:"password,omitempty"`

	// 送信方式。"gmail" は Gmail API (OAuth)、"smtp" はアプリパスワードでの SMTP。
	Transport    string `json:"transport"`
	SMTPHost     string `json:"smtpHost"`
	SMTPPort     int    `json:"smtpPort"`
	SMTPUser     string `json:"smtpUser"`
	SMTPPassword string `json:"smtpPassword,omitempty"`
}

// 送信方式の識別子。
const (
	TransportGmail = "gmail"
	TransportSMTP  = "smtp"
)

// DefaultTo は JPIRR の自動処理システムの宛先。
const DefaultTo = "auto-dbm@nic.ad.jp"

// Dir は設定ディレクトリ ~/.config/jpnic-irr-mailer を返す。
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "jpnic-irr-mailer"), nil
}

// Path は設定ファイルのパスを返す。
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load は設定を読む。ファイルが無い場合は既定値を返す。
func Load() (*Config, error) {
	c := &Config{}
	c.applyDefaults()
	p, err := Path()
	if err != nil {
		return c, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		// 初回起動は GCP 不要の SMTP を既定にする。
		// 既存の設定ファイルに transport が無い場合は、以前の挙動どおり Gmail API のまま。
		c.Transport = TransportSMTP
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("設定の読み込みに失敗: %w", err)
	}
	if err := json.Unmarshal(b, c); err != nil {
		return c, fmt.Errorf("設定の解析に失敗: %w", err)
	}
	c.applyDefaults()
	return c, nil
}

// Save は設定を 0600 で書き出す。
func (c *Config) Save() error {
	c.applyDefaults()
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), append(b, '\n'), 0o600)
}

func (c *Config) applyDefaults() {
	if c.To == "" {
		c.To = DefaultTo
	}
	if c.WhoisServer == "" {
		c.WhoisServer = "jpirr.nic.ad.jp:43"
	}
	if c.Transport == "" {
		c.Transport = TransportGmail
	}
	if c.SMTPHost == "" {
		c.SMTPHost = "smtp.gmail.com"
	}
	if c.SMTPPort == 0 {
		c.SMTPPort = 587
	}
}
