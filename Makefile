.PHONY: gen css templ sqlc build run test vet check up

gen: css templ sqlc

css:
	./internal/assets/build-css.sh

templ:
	templ generate ./internal/ui

sqlc:
	sqlc generate

build: gen
	go build -o bin/server ./cmd/server
	go build -o bin/seed-foods ./cmd/seed-foods

run: build
	./bin/server

test:
	go test ./...

vet:
	go vet ./...

check: gen vet test

up:
	podman compose up -d --build
