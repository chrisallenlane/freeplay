# Stage 1: Build
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -mod vendor -trimpath -o dist/freeplay ./cmd/freeplay

# Stage 2: Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/dist/freeplay /usr/local/bin/freeplay
RUN mkdir -p /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:8080/api/health || exit 1
ENTRYPOINT ["freeplay", "-data", "/data"]
