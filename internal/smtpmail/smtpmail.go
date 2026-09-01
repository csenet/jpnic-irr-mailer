// Package smtpmail は SMTP (STARTTLS または SMTPS) で組み立て済みのメールを送る。
// Gmail ならアプリパスワードで使え、GCP の OAuth クライアントが要らない。
package smtpmail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"time"
)

// Config は SMTP の接続先と認証情報。
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
}

// Send は raw (RFC 5322 形式) を from から to へ送る。
// 465 番は最初から TLS、それ以外は STARTTLS を必須にする。平文では認証情報を流さない。
func Send(ctx context.Context, cfg Config, from, to string, raw []byte) error {
	if cfg.Host == "" || cfg.Username == "" || cfg.Password == "" {
		return fmt.Errorf("SMTP のホスト・ユーザー・パスワードを設定してください")
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	tlsCfg := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}

	d := net.Dialer{Timeout: 30 * time.Second}
	var conn net.Conn
	var err error
	if cfg.Port == 465 {
		conn, err = tls.DialWithDialer(&d, "tcp", addr, tlsCfg)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("SMTP サーバ %s への接続に失敗: %w", addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))

	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("SMTP セッションの開始に失敗: %w", err)
	}
	defer c.Close()

	if cfg.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("SMTP サーバが STARTTLS に対応していません (平文では送りません)")
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("STARTTLS に失敗: %w", err)
		}
	}
	if err := c.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
		return fmt.Errorf("SMTP 認証に失敗 (アプリパスワードを確認してください): %w", err)
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM が拒否されました: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("RCPT TO が拒否されました: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("本文の送信に失敗: %w", err)
	}
	return c.Quit()
}
