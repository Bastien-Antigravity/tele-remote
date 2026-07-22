# === BUILD STAGE ===
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev ca-certificates tzdata

WORKDIR /workspace

# Copy required local modules for 'replace' directives
COPY microservice-toolbox ./microservice-toolbox
COPY universal-logger ./universal-logger
COPY distributed-config ./distributed-config
COPY safe-socket ./safe-socket
COPY flexible-logger ./flexible-logger

# Copy the target service
COPY tele-remote ./tele-remote

WORKDIR /workspace/tele-remote

# Ensure dependencies are tidy for linux build
RUN go mod tidy && go mod download

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /tele-remote-bin ./cmd/tele-remote

# === RUNTIME STAGE ===
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /tele-remote

# Copy the binary from the build stage
COPY --from=builder /tele-remote-bin /tele-remote/tele-remote

# Set the entrypoint
ENTRYPOINT ["/tele-remote/tele-remote"]
