FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Generated code (templ views, sqlc queries) is gitignored, so regenerate it.
RUN go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate \
 && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate \
 && CGO_ENABLED=0 go build -o /out/server ./cmd/server

FROM alpine:3.21
RUN adduser -D -H app
WORKDIR /app
COPY --from=build /out/server ./server
# Static assets are built ahead of time (Tailwind CLI + vendored HTMX).
COPY web/static ./web/static
USER app
EXPOSE 8080
CMD ["./server"]
