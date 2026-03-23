# 阶段1：基于构建机平台交叉编译 Go 二进制
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS backend
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w -X main.version=${VERSION}" -o weclaw-proxy ./cmd/weclaw-proxy/

# 阶段2：最终镜像
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=backend /app/weclaw-proxy /usr/local/bin/weclaw-proxy

# 创建数据目录
RUN mkdir -p /data

EXPOSE 8080

ENTRYPOINT ["weclaw-proxy"]
CMD ["--config", "/data/config.yaml"]
