// Package server はローカル Web UI と、その裏で動く JSON API を提供する。
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/csenet/jpnic-irr-mailer/internal/config"
	"github.com/csenet/jpnic-irr-mailer/internal/gmailer"
	"github.com/csenet/jpnic-irr-mailer/internal/rpsl"
	"github.com/csenet/jpnic-irr-mailer/internal/smtpmail"
	"github.com/csenet/jpnic-irr-mailer/internal/submit"
	"github.com/csenet/jpnic-irr-mailer/internal/whois"
)

// Server はローカル専用の HTTP サーバ。
type Server struct {
	mu         sync.Mutex
	cfg        *config.Config
	confDir    string
	baseURL    string
	gmail      *gmailer.Client
	gmailErr   error
	oauthState string
}

// New はサーバを組み立てる。credentials.json が無い場合もエラーにはせず、
// 画面側に設定手順を出せるよう gmailErr に控えておく。
func New(cfg *config.Config, confDir, baseURL string) *Server {
	s := &Server{cfg: cfg, confDir: confDir, baseURL: baseURL}
	s.gmail, s.gmailErr = gmailer.New(confDir, baseURL+"/oauth2/callback")
	return s
}

// Handler はルーティング済みの http.Handler を返す。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /", staticHandler())
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("POST /api/config", s.handleSaveConfig)
	mux.HandleFunc("GET /api/objects", s.handleObjects)
	mux.HandleFunc("POST /api/preview", s.handlePreview)
	mux.HandleFunc("POST /api/send", s.handleSend)
	mux.HandleFunc("GET /api/auth/url", s.handleAuthURL)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("POST /api/smtp/clear", s.handleClearSMTPPassword)
	mux.HandleFunc("GET /oauth2/callback", s.handleCallback)
	return localOnly(mux)
}

// localOnly は DNS rebinding 対策として Host ヘッダをループバックに限定する。
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		switch host {
		case "localhost", "127.0.0.1", "::1", "[::1]":
		default:
			http.Error(w, "このツールは localhost からのみ利用できます", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

// --- 状態と設定 ---

type stateResp struct {
	Config     *config.Config `json:"config"`
	Authorized bool           `json:"authorized"`
	Account    string         `json:"account"`
	GmailSetup string         `json:"gmailSetup"` // credentials.json が無いときの案内
	ConfigDir  string         `json:"configDir"`
	// SMTP パスワードは応答に載せないので、設定済みかどうかだけ伝える。
	SMTPPasswordSet bool `json:"smtpPasswordSet"`
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	cfg := *s.cfg
	s.mu.Unlock()

	// 応答には SMTP パスワードを含めない (画面側は「保存済み」表示だけで足りる)。
	// 判定に使う値は消す前に控えておく。
	smtpReady := cfg.SMTPUser != "" && cfg.SMTPPassword != ""
	public := cfg
	public.SMTPPassword = ""
	resp := stateResp{Config: &public, ConfigDir: s.confDir, SMTPPasswordSet: cfg.SMTPPassword != ""}

	if cfg.Transport == config.TransportSMTP {
		resp.Authorized = smtpReady
		resp.Account = cfg.SMTPUser
		if resp.Authorized && cfg.From == "" {
			s.mu.Lock()
			s.cfg.From = cfg.SMTPUser
			resp.Config.From = cfg.SMTPUser
			_ = s.cfg.Save()
			s.mu.Unlock()
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if s.gmailErr != nil {
		resp.GmailSetup = s.gmailErr.Error()
	} else if s.gmail.Authorized() {
		resp.Authorized = true
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if addr, err := s.gmail.Address(ctx); err == nil {
			resp.Account = addr
			// 送信元が未設定なら認証アカウントを既定値にする。
			s.mu.Lock()
			if s.cfg.From == "" {
				s.cfg.From = addr
				resp.Config.From = addr
				_ = s.cfg.Save()
			}
			s.mu.Unlock()
		} else {
			resp.Authorized = false
			resp.GmailSetup = "保存済みトークンが使えませんでした。もう一度ログインしてください: " + err.Error()
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var in config.Config
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Mntner = strings.TrimSpace(in.Mntner)
	s.cfg.From = strings.TrimSpace(in.From)
	s.cfg.ChangedBy = strings.TrimSpace(in.ChangedBy)
	s.cfg.To = strings.TrimSpace(in.To)
	s.cfg.WhoisServer = strings.TrimSpace(in.WhoisServer)
	s.cfg.Notify = strings.TrimSpace(in.Notify)
	s.cfg.DefaultOrigin = strings.TrimSpace(in.DefaultOrigin)
	s.cfg.Password = in.Password
	s.cfg.Transport = strings.TrimSpace(in.Transport)
	s.cfg.SMTPHost = strings.TrimSpace(in.SMTPHost)
	s.cfg.SMTPPort = in.SMTPPort
	s.cfg.SMTPUser = strings.TrimSpace(in.SMTPUser)
	// 空で送られてきたら「変更なし」。消したいときは画面のクリアボタンで明示する。
	if in.SMTPPassword != "" {
		s.cfg.SMTPPassword = in.SMTPPassword
	}
	if err := s.cfg.Save(); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := *s.cfg
	out.SMTPPassword = ""
	writeJSON(w, http.StatusOK, out)
}

// handleClearSMTPPassword は保存済みの SMTP パスワードを消す。
func (s *Server) handleClearSMTPPassword(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.SMTPPassword = ""
	if err := s.cfg.Save(); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- whois 取得 ---

type objectsResp struct {
	Objects []rpsl.Object `json:"objects"`
	Server  string        `json:"server"`
}

func (s *Server) handleObjects(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	mntner := s.cfg.Mntner
	server := s.cfg.WhoisServer
	s.mu.Unlock()

	if q := strings.TrimSpace(r.URL.Query().Get("mntner")); q != "" {
		mntner = q
	}
	if mntner == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("Maintainer 名を設定してください (例: MAINT-AS64496)"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	objs, err := whois.New(server).ByMaintainer(ctx, mntner)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, objectsResp{Objects: objs, Server: server})
}

// --- プレビューと送信 ---

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	req, err := s.decodeSubmit(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	res, err := submit.Build(*req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// プレビュー応答には実本文 (パスワード入り) を含めない。
	res.Body = ""
	writeJSON(w, http.StatusOK, res)
}

type sendResp struct {
	MessageID string   `json:"messageId"`
	To        string   `json:"to"`
	Subject   string   `json:"subject"`
	Warnings  []string `json:"warnings"`
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	req, err := s.decodeSubmit(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	res, err := submit.Build(*req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.mu.Lock()
	cfg := *s.cfg
	s.mu.Unlock()
	to := cfg.To
	if to == "" {
		to = config.DefaultTo
	}
	msg := gmailer.Message{From: cfg.From, To: to, Subject: res.Subject, Body: res.Body}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	var id string
	if cfg.Transport == config.TransportSMTP {
		raw, err := msg.Build()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		from := cfg.From
		if from == "" {
			from = cfg.SMTPUser
		}
		if err := smtpmail.Send(ctx, smtpmail.Config{
			Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUser, Password: cfg.SMTPPassword,
		}, from, to, raw); err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		id = "smtp"
	} else {
		if s.gmailErr != nil {
			writeErr(w, http.StatusPreconditionFailed, s.gmailErr)
			return
		}
		var err error
		id, err = s.gmail.Send(ctx, msg)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
	}
	log.Printf("送信しました: %d 件の変更を %s へ (message id %s)", len(req.Changes), to, id)
	writeJSON(w, http.StatusOK, sendResp{MessageID: id, To: to, Subject: res.Subject, Warnings: res.Warnings})
}

// decodeSubmit はリクエストを読み、From が空なら設定値で補う。
func (s *Server) decodeSubmit(r *http.Request) (*submit.Request, error) {
	var req submit.Request
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20)).Decode(&req); err != nil {
		return nil, fmt.Errorf("リクエストの解析に失敗: %w", err)
	}
	s.mu.Lock()
	// submit 側の From は changed:/delete: に書くアドレス。
	// 画面から来なければ設定の ChangedBy、それも無ければメールの From を使う。
	if strings.TrimSpace(req.From) == "" {
		req.From = s.cfg.ChangedBy
	}
	if strings.TrimSpace(req.From) == "" {
		req.From = s.cfg.From
	}
	if req.Password == "" {
		req.Password = s.cfg.Password
	}
	s.mu.Unlock()
	return &req, nil
}

// --- OAuth ---

func (s *Server) handleAuthURL(w http.ResponseWriter, r *http.Request) {
	if s.gmailErr != nil {
		writeErr(w, http.StatusPreconditionFailed, s.gmailErr)
		return
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	state := hex.EncodeToString(b)
	s.mu.Lock()
	s.oauthState = state
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"url": s.gmail.AuthCodeURL(state)})
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	if s.gmailErr != nil {
		httpPage(w, http.StatusPreconditionFailed, "設定が未完了です", s.gmailErr.Error())
		return
	}
	if e := r.URL.Query().Get("error"); e != "" {
		httpPage(w, http.StatusBadRequest, "認可されませんでした", e)
		return
	}
	s.mu.Lock()
	want := s.oauthState
	s.oauthState = ""
	s.mu.Unlock()

	got := r.URL.Query().Get("state")
	if want == "" || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
		httpPage(w, http.StatusBadRequest, "state が一致しません", "画面を開き直してもう一度ログインしてください。")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		httpPage(w, http.StatusBadRequest, "認可コードがありません", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.gmail.Exchange(ctx, code); err != nil {
		httpPage(w, http.StatusBadGateway, "トークンの取得に失敗しました", err.Error())
		return
	}
	httpPage(w, http.StatusOK, "ログインできました", "このタブは閉じて、元の画面に戻ってください。")
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.gmail == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if err := s.gmail.Logout(); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// httpPage は OAuth コールバック用の素朴な HTML を返す。
func httpPage(w http.ResponseWriter, code int, title, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>%s</title>
<body style="font-family:system-ui;padding:3rem;line-height:1.7">
<h1 style="font-size:1.3rem">%s</h1><p>%s</p></body>`,
		html.EscapeString(title), html.EscapeString(title), html.EscapeString(detail))
}
