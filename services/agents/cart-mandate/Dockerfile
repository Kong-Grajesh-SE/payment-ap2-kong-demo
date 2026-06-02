FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
COPY ap2/ ./ap2/
COPY cart-mandate/ ./cart-mandate/
RUN go mod tidy && CGO_ENABLED=0 go build -o /cart-mandate-agent ./cart-mandate/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /cart-mandate-agent /usr/local/bin/cart-mandate-agent
EXPOSE 9003
ENTRYPOINT ["/usr/local/bin/cart-mandate-agent"]
