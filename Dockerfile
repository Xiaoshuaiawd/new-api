FROM oven/bun:latest AS builder

# 设置环境变量避免 esbuild 验证问题
ENV ESBUILD_BINARY_PATH=/usr/local/bin/esbuild
ENV CI=true

WORKDIR /build
COPY web/package.json .

# 跳过 postinstall 脚本安装依赖
RUN bun install --ignore-scripts

# 手动处理 esbuild（如果需要）
RUN bun add esbuild@0.21.5 --ignore-scripts || echo "esbuild handled"

COPY ./web .
COPY ./VERSION .
RUN DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat VERSION) bun run build

FROM golang:1.25.1 AS builder2

ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /build

ADD go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=builder /build/dist ./web/dist
RUN go build -ldflags "-s -w -X 'one-api/common.Version=$(cat VERSION)'" -o one-api

FROM alpine

RUN apk upgrade --no-cache \
    && apk add --no-cache ca-certificates tzdata ffmpeg \
    && update-ca-certificates

COPY --from=builder2 /build/one-api /
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/one-api"]
