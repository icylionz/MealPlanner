FROM docker.io/library/golang:1.26-alpine AS build
WORKDIR /src
# build-css.sh downloads the standalone Tailwind CLI (glibc binary; gcompat +
# libstdc++ let it run on musl).
RUN apk add --no-cache curl ca-certificates gcompat libstdc++
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Generated code (templ views, sqlc queries, Tailwind CSS) is not committed.
RUN ./internal/assets/build-css.sh \
 && go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate ./internal/view \
 && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate \
 && CGO_ENABLED=0 go build -trimpath -o /out/server ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -o /out/seed-foods ./cmd/seed-foods

FROM docker.io/library/alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H app
WORKDIR /app
COPY --from=build /out/server /out/seed-foods ./
USER app
EXPOSE 8080
CMD ["./server"]
