FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
COPY ap2/ ./ap2/
COPY cart-intent/ ./cart-intent/
RUN go mod tidy && CGO_ENABLED=0 go build -o /cart-intent-agent ./cart-intent/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /cart-intent-agent /usr/local/bin/cart-intent-agent
EXPOSE 9002
ENTRYPOINT ["/usr/local/bin/cart-intent-agent"]
