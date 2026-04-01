# === BUILD STAGE ===
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev ca-certificates tzdata

WORKDIR /tele-remote

# Copy dependency files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /tele-remote/tele-remote ./cmd/tele-remote

# === RUNTIME STAGE ===
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /tele-remote

# Copy the binary from the build stage
COPY --from=builder /tele-remote/tele-remote /tele-remote/tele-remote

# Set the entrypoint
ENTRYPOINT ["/tele-remote/tele-remote"]