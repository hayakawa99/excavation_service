# ビルドステージ
FROM golang:1.24-alpine AS builder 

WORKDIR /app

RUN apk add --no-cache git 

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# アプリケーションをビルド
# バイナリ名は excavation_service に合わせましょう
RUN CGO_ENABLED=0 GOOS=linux go build -o excavation_service ./cmd/api/main.go


# 実行ステージ
FROM alpine:latest 

RUN apk --no-cache add ca-certificates 

WORKDIR /root/ 

# ビルドステージで作成したバイナリをコピー
# --from=builder でビルドステージを指定
COPY --from=builder /app/excavation_service . 
# アプリケーションを実行
CMD ["./excavation_service"]
EXPOSE 18080 