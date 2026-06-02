FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
COPY ap2/ ./ap2/
COPY search/ ./search/
RUN go mod tidy && CGO_ENABLED=0 go build -o /search-agent ./search/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /search-agent /usr/local/bin/search-agent
EXPOSE 9001
ENTRYPOINT ["/usr/local/bin/search-agent"]
