FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
COPY ap2/ ./ap2/
COPY payment/ ./payment/
RUN go mod tidy && CGO_ENABLED=0 go build -o /payment-agent ./payment/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /payment-agent /usr/local/bin/payment-agent
EXPOSE 9004
ENTRYPOINT ["/usr/local/bin/payment-agent"]
