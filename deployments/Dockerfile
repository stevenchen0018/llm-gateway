FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/gateway ./cmd/gateway \
 && CGO_ENABLED=0 go build -trimpath -o /out/migrate ./cmd/migrate \
 && CGO_ENABLED=0 go build -trimpath -o /out/seed ./cmd/seed

FROM alpine:3.20
RUN adduser -D -u 10001 gateway
# SQL migrations are embedded in the binaries (migrations/embed.go): no files to copy,
# and the gateway migrates the database itself on startup (postgres.auto_migrate).
COPY --from=build /out/gateway /usr/local/bin/gateway
COPY --from=build /out/migrate /usr/local/bin/migrate
COPY --from=build /out/seed /usr/local/bin/seed
USER gateway
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/gateway"]
