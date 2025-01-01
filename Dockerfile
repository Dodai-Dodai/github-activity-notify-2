# ビルドステージ
FROM golang:1.23.3 AS builder

# 作業ディレクトリを設定
WORKDIR /app

# モジュールファイルをコピーして依存関係を取得
COPY go.mod go.sum ./
RUN go mod tidy && go mod download

# アプリケーションコードをコピー
COPY . .

# 環境変数を設定してクロスコンパイル
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -o /main main.go

# 実行ステージ
FROM gcr.io/distroless/static-debian12:latest

# ビルド済みバイナリをコピー
COPY --from=builder /main /main

# 実行可能ファイルをエントリーポイントに設定
ENTRYPOINT ["/main"]
