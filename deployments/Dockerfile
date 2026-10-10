# syntax=docker/dockerfile:1
# One image = gateway API + console (web/dist served by the gateway itself).
# Build from the repository root:
#   docker build -f deployments/Dockerfile -t llm-gateway:latest .

# ---- console --------------------------------------------------------------
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- gateway binaries -------------------------------------------------------
FROM golang:1.26-alpine AS build
WORKDIR /src
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/gateway ./cmd/gateway \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/migrate ./cmd/migrate \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/seed ./cmd/seed

# ---- runtime ----------------------------------------------------------------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget \
 && adduser -D -u 10001 gateway
ENV TZ=Asia/Shanghai \
    LLM_GATEWAY_SERVER_WEB_ROOT=/app/web \
    LLM_GATEWAY_LOG_FORMAT=json \
    GIN_MODE=release
WORKDIR /app
# SQL migrations are embedded in the binaries (migrations/embed.go); the master
# node migrates the database itself on startup (postgres.auto_migrate).
COPY --from=build /out/gateway /usr/local/bin/gateway
COPY --from=build /out/migrate /usr/local/bin/migrate
COPY --from=build /out/seed /usr/local/bin/seed
COPY --from=web /web/dist /app/web
USER gateway
EXPOSE 8080
# liveness only; load balancers should probe /readyz (DB + Redis + schema)
HEALTHCHECK --interval=15s --timeout=3s --start-period=20s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/gateway"]
