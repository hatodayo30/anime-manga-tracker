FROM node:22-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.25-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
# migrations/ は embed.FS でバイナリに焼き込むため、ビルド時に必要
COPY migrations/ migrations/
RUN CGO_ENABLED=0 go build -o /app/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
# 非 root で動かす（アプリのプロセスと docker exec の既定ユーザーがこれになる）。
# UID を固定するのは、runAsNonRoot 相当の検査が名前ではなく数値で判定するため。
# ホームディレクトリは不要（-H）。何も書き込まないプロセスなので。
RUN addgroup -g 10001 app && adduser -D -H -u 10001 -G app app
WORKDIR /app
# 所有者は root のまま（chown しない）。読み取りと実行には COPY 既定の 644 / 755 で足り、
# アプリ自身がバイナリや静的ファイルを書き換えられない方が安全。
# マイグレーションの SQL は embed.FS でバイナリに入っているので、ファイルとしては読まない。
COPY --from=go-builder /app/server ./server
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist

# 8080 は非特権ポートなので、非 root でも bind できる
EXPOSE 8080
USER 10001:10001
ENTRYPOINT ["./server"]
