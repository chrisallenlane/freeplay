# Stage 1: Download EmulatorJS
FROM alpine AS emulatorjs
RUN apk add --no-cache curl p7zip
ARG EMULATORJS_VERSION=4.2.3
RUN curl -L "https://github.com/EmulatorJS/EmulatorJS/releases/download/v${EMULATORJS_VERSION}/${EMULATORJS_VERSION}.7z" \
    -o /tmp/emulatorjs.7z && \
    7z x /tmp/emulatorjs.7z -o/tmp/emulatorjs -y > /dev/null && \
    rm /tmp/emulatorjs.7z

# Stage 2: Build
FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=emulatorjs /tmp/emulatorjs/ ./emulatorjs/
RUN CGO_ENABLED=0 go build -o dist/freeplay ./cmd/freeplay

# Stage 3: Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/dist/freeplay /usr/local/bin/freeplay
RUN mkdir -p /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:8080/api/health || exit 1
ENTRYPOINT ["freeplay", "-data", "/data"]
