FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /ezra ./cmd/ezra

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /ezra /usr/local/bin/ezra
COPY migrations /migrations
COPY config /config
EXPOSE 8080
CMD ["ezra"]
