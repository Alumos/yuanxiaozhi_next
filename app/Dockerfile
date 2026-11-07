# 前端与平台无关：在构建机原生架构上跑；Go 交叉编译到目标架构（amd64 / arm64）
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN mkdir -p ../server/internal/core && npx vue-tsc --noEmit && npx vite build

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS srv
ARG TARGETOS TARGETARCH
WORKDIR /src/server
COPY server/ ./
COPY --from=web /src/server/internal/core/dist ./internal/core/dist
RUN go vet ./... && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /yxz ./cmd/yxz

FROM alpine:3.22
RUN adduser -D yxz && apk add --no-cache tzdata ca-certificates && mkdir /data && chown yxz /data
COPY --from=srv /yxz /app/yxz
COPY content/ /app/content/
USER yxz
ENV YXZ_PORT=8080 YXZ_DATA=/data TZ=Asia/Shanghai
EXPOSE 8080
VOLUME /data
HEALTHCHECK CMD wget -qO- http://127.0.0.1:8080/api/common/health || exit 1
ENTRYPOINT ["/app/yxz", "serve", "-content", "/app/content", "-env", "/nonexistent"]
