FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -o /worm-storage .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /worm-storage /usr/local/bin/worm-storage
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/worm-storage"]
