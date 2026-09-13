# === BUILD STAGE ===
FROM golang:1.25-alpine AS builder

LABEL org.opencontainers.image.source="https://github.com/Bastien-Antigravity/tele-remote"

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev ca-certificates tzdata

WORKDIR /workspace

# Clone shared library modules for replace directives in builder stage
RUN git clone --depth 1 https://github.com/Bastien-Antigravity/microservice-toolbox.git /workspace/microservice-toolbox && \
    git clone --depth 1 https://github.com/Bastien-Antigravity/distributed-config.git /workspace/distributed-config && \
    git clone --depth 1 https://github.com/Bastien-Antigravity/safe-socket.git /workspace/safe-socket && \
    git clone --depth 1 https://github.com/Bastien-Antigravity/universal-logger.git /workspace/universal-logger && \
    git clone --depth 1 https://github.com/Bastien-Antigravity/flexible-logger.git /workspace/flexible-logger

# Copy tele-remote source
WORKDIR /workspace/tele-remote
COPY . .

# Ensure dependencies are tidy and build binary
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /tele-remote-bin ./cmd/tele-remote

# === RUNTIME STAGE ===
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /tele-remote

# Copy the binary from the build stage
COPY --from=builder /tele-remote-bin /tele-remote/tele-remote

# Set the entrypoint
ENTRYPOINT ["/tele-remote/tele-remote"]

