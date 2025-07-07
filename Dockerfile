# Stage 1: Build the Go application
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache make

WORKDIR /app

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the application using Makefile
RUN make build

# Stage 2: Create the final image
FROM alpine:latest

WORKDIR /app

# Copy the built application from the builder stage
COPY --from=builder /app/cmd/gogemini/gogemini .
# Expose the port the application runs on
EXPOSE ${GOGEMINI_PORT}

# Run the application
CMD ["/app/gogemini"]