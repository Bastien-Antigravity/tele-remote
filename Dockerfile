# === BUILD STAGE ===
FROM golang:1.25-alpine AS builder

LABEL org.opencontainers.image.source="https://github.com/Bastien-Antigravity/tele-remote"

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev ca-certificates tzdata

WORKDIR /workspace

# Clone shared library modules for go.work resolution in builder stage
RUN git clone --depth 1 -b develop https://github.com/Bastien-Antigravity/microservice-toolbox.git /workspace/microservice-toolbox && \
    git clone --depth 1 -b develop https://github.com/Bastien-Antigravity/distributed-config.git /workspace/distributed-config && \
    git clone --depth 1 -b develop https://github.com/Bastien-Antigravity/safe-socket.git /workspace/safe-socket && \
    git clone --depth 1 -b develop https://github.com/Bastien-Antigravity/universal-logger.git /workspace/universal-logger && \
    git clone --depth 1 -b develop https://github.com/Bastien-Antigravity/flexible-logger.git /workspace/flexible-logger

# Copy tele-remote source
WORKDIR /workspace/tele-remote
COPY . .

# Initialize workspace go.work for cloned sibling modules and build binary
RUN cd /workspace && \
    go work init ./tele-remote ./microservice-toolbox ./distributed-config ./safe-socket ./universal-logger ./flexible-logger && \
    cd /workspace/tele-remote && \
    VERSION=$(cat VERSION.txt) && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X 'github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/version.BuildVersion=${VERSION}'" -o /tele-remote-bin ./cmd/tele-remote

# === RUNTIME STAGE ===
FROM alpine:3.20

LABEL org.opencontainers.image.source="https://github.com/Bastien-Antigravity/tele-remote"

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /tele-remote

# Copy the binary and VERSION.txt from the build stage
COPY --from=builder /tele-remote-bin /tele-remote/tele-remote
COPY --from=builder /workspace/tele-remote/VERSION.txt /tele-remote/VERSION.txt

# Set the entrypoint
ENTRYPOINT ["/tele-remote/tele-remote"]

