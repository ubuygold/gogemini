# Stage 1: Build the Go application
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the frontend
WORKDIR /app/frontend
RUN npm install -g bun
RUN bun install
RUN bun run build

# Build the Go application
WORKDIR /app
RUN CGO_ENABLED=0 GOOS=linux go build -o /gogemini ./cmd/gogemini

# Stage 2: Create the final image
FROM alpine:latest

WORKDIR /app

# Copy the built application from the builder stage
COPY --from=builder /gogemini .

# Copy the config file
COPY config.yaml .

# Expose the port the application runs on
EXPOSE 8081

# Run the application
CMD ["/app/gogemini"]