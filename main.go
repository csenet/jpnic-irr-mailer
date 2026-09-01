// jpnic-irr-mailer は JPIRR の登録内容を whois から取り込み、
// 画面で編集した差分を auto-dbm@nic.ad.jp 宛の plain text メールとして送るツール。
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/csenet/jpnic-irr-mailer/internal/config"
	"github.com/csenet/jpnic-irr-mailer/internal/server"
)

func main() {
	// OAuth のリダイレクト URI は GCP 側に登録する必要があるため、既定ポートは固定にする。
	port := flag.Int("port", 8787, "待ち受けポート (OAuth のリダイレクト URI と揃える必要があります)")
	open := flag.Bool("open", true, "起動時にブラウザを開く")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Printf("警告: %v (既定値で起動します)", err)
	}
	confDir, err := config.Dir()
	if err != nil {
		log.Fatalf("設定ディレクトリを決められませんでした: %v", err)
	}
	if err := os.MkdirAll(confDir, 0o700); err != nil {
		log.Fatalf("設定ディレクトリを作成できませんでした: %v", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	baseURL := fmt.Sprintf("http://localhost:%d", *port)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("%s を待ち受けできませんでした: %v", addr, err)
	}

	srv := &http.Server{
		Handler:           server.New(cfg, confDir, baseURL).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	fmt.Printf("JPIRR Mailer を起動しました\n")
	fmt.Printf("  UI            : %s\n", baseURL)
	fmt.Printf("  設定と認証情報 : %s\n", confDir)
	fmt.Printf("  リダイレクトURI: %s/oauth2/callback\n", baseURL)
	fmt.Printf("終了するには Ctrl-C を押してください。\n")

	if *open {
		go openBrowser(baseURL)
	}
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// openBrowser は既定のブラウザで UI を開く。失敗しても致命的ではない。
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	if err := exec.Command(cmd, append(args, url)...).Start(); err != nil {
		log.Printf("ブラウザを開けませんでした (%v)。手動で %s を開いてください。", err, url)
	}
}
