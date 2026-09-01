# jpnic-irr-mailer

JPNIC の [JPIRR](https://www.nic.ad.jp/ja/ip/irr/) に登録済みのオブジェクトを whois から取り込み、
ブラウザ上で編集した差分を `auto-dbm@nic.ad.jp` 宛の plain text メールとして Gmail から送るツールです。

- 現在の登録内容を **whois (`jpirr.nic.ad.jp:43`) から取得** して一覧表示
- route / route6 の **追加・変更・削除** を画面で編集
- person / role（JPNIC ハンドルの連絡先）、aut-num、as-set の新規作成・編集にも対応
- 内容を変えずに `changed:` の日付だけ更新する **再登録**（ガベージコレクタ対策）
- 最終更新からの経過月数を一覧に表示し、**自動削除が近いものを警告**
- 送信前に **実際のメール本文 (RPSL) をプレビュー**（`password` は伏字）
- **Gmail API (OAuth2)** で plain text メールとして送信

JPIRR の自動処理は HTML メールを受け付けないため、本ツールは常に `text/plain` 単体で送信します。

UI は Bootstrap 5.3 ですが、CSS/JS はリポジトリに同梱して `go:embed` しているため、
CDN への接続は不要です。ビルドは `go build` だけで完結し、Node は要りません。

## インストール

**バイナリ** — [Releases](https://github.com/csenet/jpnic-irr-mailer/releases) から OS に合うアーカイブを取って展開するだけです（macOS / Linux / Windows、amd64 / arm64）。

```
tar xzf jpnic-irr-mailer_*_darwin_arm64.tar.gz
./jpnic-irr-mailer
```

**go install** — Go 1.22 以降があるなら:

```
go install github.com/csenet/jpnic-irr-mailer@latest
```

**ソースから**:

```
git clone https://github.com/csenet/jpnic-irr-mailer && cd jpnic-irr-mailer
go run .
```

起動すると `http://localhost:8787` がブラウザで開きます。

## 最短セットアップ（SMTP + アプリパスワード）

GCP の設定なしで使う一番簡単な方法です。

1. Google アカウントで [2 段階認証プロセス](https://myaccount.google.com/signinoptions/two-step-verification)を有効にする（既に有効なら不要）
2. [アプリ パスワード](https://myaccount.google.com/apppasswords)で「JPIRR Mailer」など適当な名前で発行し、16 桁をコピー
3. ツールの「設定」→「送信方式」で **SMTP + アプリパスワード** を選び、ユーザー名（Gmail アドレス）とアプリパスワードを入れて保存
4. 「Maintainer」に自分の `MAINT-ASxxxx` を入れて保存 →「JPIRR から取得」

アプリパスワードは `config.json`（`0600`）に保存されます。「クリア」ボタンで消せます。
Gmail 以外の SMTP サーバも指定できます（465 番は SMTPS、それ以外は STARTTLS 必須。平文では送りません）。

## 使い方

オプション:

| フラグ | 既定値 | 説明 |
| --- | --- | --- |
| `-port` | `8787` | 待ち受けポート。OAuth のリダイレクト URI と揃える必要があります |
| `-open` | `true` | 起動時にブラウザを開く |

待ち受けは `127.0.0.1` のみ、かつ Host ヘッダを localhost に限定しています。

## Gmail API (OAuth) で送る場合

アプリパスワードを使いたくない場合は、Google Cloud で作った OAuth クライアントでも送れます。
「設定」→「送信方式」で **Gmail API + OAuth** を選び、以下を一度だけ行ってください。

1. [Google Cloud Console](https://console.cloud.google.com/) でプロジェクトを作る（既存のものでも可）
2. 「API とサービス」→「ライブラリ」で **Gmail API** を有効化
3. 「OAuth 同意画面」を設定（User type は自分だけなら **外部** + テストユーザーに自分を追加でよい）
4. 「認証情報」→「認証情報を作成」→ **OAuth クライアント ID** → 種類は **ウェブ アプリケーション**
5. 「承認済みのリダイレクト URI」に次を追加

   ```
   http://localhost:8787/oauth2/callback
   ```

   `-port` を変えた場合はその番号に合わせてください。
6. JSON をダウンロードし、`credentials.json` という名前で設定ディレクトリに置く

   - macOS: `~/Library/Application Support/jpnic-irr-mailer/credentials.json`
   - Linux: `~/.config/jpnic-irr-mailer/credentials.json`

   正確なパスは起動時のログと画面上部に表示されます。
7. ツールを起動し、画面右上の **Google でログイン** を押して同意

要求するスコープは `gmail.send`（送信のみ）と `userinfo.email`（画面に出す自分のアドレスの取得）だけです。
メールの閲覧権限は要求しません。取得したトークンは同じディレクトリに `token.json` として `0600` で保存されます。

## 設定項目

画面の「設定」パネルで指定し、`config.json` に保存されます。

| 項目 | 内容 |
| --- | --- |
| Maintainer (mnt-by) | 取得対象の maintainer。例: `MAINT-AS64496` |
| メールの From | Gmail で送るときの `From`。認証アカウントを使う |
| changed: に書くアドレス | オブジェクトの `changed:` / `delete:` に記録する連絡先。空なら From と同じ |
| 宛先 | 既定は `auto-dbm@nic.ad.jp` |
| whois サーバ | 既定は `jpirr.nic.ad.jp:43` |
| notify / origin の既定値 | 新規オブジェクト作成時の初期値 |

| JPIRR パスワード (任意) | 保存すると送信ダイアログに自動で入ります。空なら送信のたびに入力 |
| 送信方式 | SMTP + アプリパスワード、または Gmail API + OAuth |
| SMTP サーバ / ポート / ユーザー名 / アプリパスワード | SMTP 方式のときの接続先。既定は `smtp.gmail.com:587` |

パスワードを保存する場合、`config.json` に平文で書かれます（ファイルは `0600`）。
共用マシンや同期対象のディレクトリでは空のままにして、送信のたびに入力する運用を勧めます。

## 動作の詳細

### 取得

`-s JPIRR -i mnt-by <MAINTAINER>` で whois に問い合わせ、返ってきた RPSL をそのままパースします。
継続行（先頭が空白の `X-Keiro:` など）も属性値の一部として保持し、送信時に元の形へ復元します。

### 送信本文の組み立て

JPNIC の[記入例](https://www.nic.ad.jp/doc/jpnic-01077.html)にならい、
オブジェクトごとに先頭へ `password:` を置き、オブジェクト間を空行で区切って 1 通にまとめます。

- **新規 / 変更**: `changed:` を「changed 用アドレス + 当日の日付」に更新
- **削除**: 登録時の `changed:` はそのまま残し、`source:` の直前に `delete:` を追加
- `source:` は常に `JPIRR` として末尾に配置

### 再登録（ガベージコレクタ対策）

JPIRR には未更新オブジェクトの[ガベージコレクター](https://www.nic.ad.jp/ja/ip/irr/gc/)があり、
**最終更新から 12 ヶ月で更新期間満了の通知、14 ヶ月で自動削除**されます。
削除を避けるには「`changed` 属性の日付だけを変えたオブジェクトを提出する」のが JPNIC の案内する手順です。

一覧の **再登録** ボタン、またはヘッダの **表示中をまとめて再登録** がこれに当たります。
内容は一切変えず、`changed:` だけが送信元アドレスと当日の日付に書き換わります。

「最終更新」列には経過月数を表示し、11 ヶ月以上経過したものは赤字で残り期間を出します。
**更新期限が近いものだけ** にチェックを入れると対象だけに絞り込めるので、
そこで「表示中をまとめて再登録」を押せば期限切れ間近のものを一括で送り直せます。

### person / role（ハンドル）について

JPIRR にはハンドルの自動採番（RIPE の `AUTO-1` に相当するもの）が**ありません**。
`nic-hdl` には JPNIC から割り当て済みのハンドル（`TK36JP` や `JP00000000`）を書く必要があり、
ハンドルそのものの発行は JPNIC の申請システム側で行います。
本ツールでは「+ その他 → person / role」から、持っているハンドルの連絡先オブジェクトを登録・更新・削除できます。
`nic-hdl` に `AUTO-` を書いた場合は送信を止めます。

### 送信前の検証

以下は送信を止めます。

- プレフィックスが不正、またはホストビットが立っている（`192.0.2.1/24` など）
- `route` に IPv6、`route6` に IPv4 を書いている
- `origin` / `descr` / `mnt-by` が空、`origin` が AS 番号の形式でない
- person / role で `nic-hdl` / `address` / `phone` が空、または `nic-hdl` が `AUTO-`
- aut-num で `as-name` / `descr` / `admin-c` / `tech-c` が空、as-set で名前が `AS-` で始まらないか `members` が空
- `mntner` の新規登録（`irr-admin@nic.ad.jp` への申請が必要なため対象外。既存 mntner の更新・削除は送れます）

非 ASCII 文字が含まれる場合は、送信は可能ですが警告を出します（JPIRR は ASCII のみ受け付けます）。

## 注意

- 送信後は JPIRR から届く**結果通知メールで登録内容を必ず確認**してください。本ツールは送信までを担当します。
- mntner を再登録・更新するとき、whois から取り込んだ `auth: CRYPT-PW HIDDENCRYPTPW`（伏字）は
  送信時に入力したパスワードから作った CRYPT-PW ハッシュへ自動で置き換えます
  （`internal/descrypt` の crypt(3) 実装。salt は毎回乱数なのでハッシュ文字列は変わりますが、同じパスワードで認証できます）。
  PGPKEY など CRYPT-PW 以外の `auth` は触りません。置き換えたときはプレビューに警告を出します。
- `auth:` の CRYPT-PW を変更したい場合は、JPNIC の
  [CRYPT-PW 生成ツール](https://www.nic.ad.jp/ja/ip/irr/crypt-gen.html)で暗号化した文字列を使い、
  mntner オブジェクトの手続きに従ってください。

## 開発

```
go test ./...
```

| パス | 役割 |
| --- | --- |
| `internal/rpsl` | RPSL のパースと整形（継続行、属性の順序と重複を保持） |
| `internal/whois` | JPIRR whois クライアント |
| `internal/submit` | 変更セットの検証とメール本文の組み立て |
| `internal/descrypt` | 伝統的 Unix crypt(3) (CRYPT-PW 用)。libc の crypt と突き合わせるテスト付き |
| `internal/gmailer` | OAuth2 と Gmail API による送信、RFC 5322 メールの組み立て |
| `internal/smtpmail` | SMTP (STARTTLS / SMTPS) による送信 |
| `internal/server` | ローカル HTTP API と、embed した Web UI (`internal/server/web`) |
| `internal/server/web/vendor` | 同梱した Bootstrap 5.3 の CSS / JS |

## リリース

`v*` のタグを push すると GitHub Actions (GoReleaser) が各 OS のバイナリを Releases に並べます。

```
git tag v0.1.0 && git push origin v0.1.0
```

## ライセンス

MIT。同梱の Bootstrap も MIT です。
