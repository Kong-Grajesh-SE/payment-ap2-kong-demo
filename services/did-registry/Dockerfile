FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -o /did-registry .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /did-registry /usr/local/bin/did-registry
EXPOSE 8070
ENTRYPOINT ["/usr/local/bin/did-registry"]
