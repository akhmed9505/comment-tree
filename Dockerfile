FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY web/ web/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM golang:1.27.3-alpine AS goose-build
RUN CGO_ENABLED=0 go install -tags='no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb' github.com/pressly/goose/v3/cmd/goose@v3.27.0

FROM alpine:3.23 AS runtime
RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S -G app app
WORKDIR /app
USER app:app

FROM runtime AS migrate
COPY --from=goose-build /go/bin/goose /usr/local/bin/goose
COPY migrations/ /migrations/
ENTRYPOINT ["goose"]

FROM runtime AS app
COPY --from=build /out/server ./server
COPY configs/config.yaml ./configs/config.yaml
EXPOSE 8080
STOPSIGNAL SIGTERM
ENTRYPOINT ["/app/server"]
