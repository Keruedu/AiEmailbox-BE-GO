# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /app

# Copy go mod files
COPY go.mod ./
COPY go.sum* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Generate Swagger documentation with compatible version
RUN go install github.com/swaggo/swag/cmd/swag@v1.8.12 && \
    swag init -g cmd/server/main.go

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/server

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/main .

# Copy .env.example as a template (users should mount their own .env)
COPY --from=builder /app/.env.example .

# Expose port
EXPOSE 8080

# Run the application
CMD ["./main"]
