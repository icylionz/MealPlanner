FROM golang:1.23-alpine AS builder

WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
RUN go install github.com/a-h/templ/cmd/templ@latest
RUN go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
RUN ls -la
COPY go.mod go.sum ./
RUN go mod download
RUN go mod tidy

COPY . .
RUN sqlc generate
RUN templ generate 
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o app main.go

# Runtime stage  
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/app .

EXPOSE 8080
CMD ["./app"]