# Stage 1: Build the Go application
FROM golang:1.24-alpine AS builder

# Accept GOGEMINI_PORT as a build argument
ARG GOGEMINI_PORT
# Set it as an environment variable for the builder stage
ENV GOGEMINI_PORT=${GOGEMINI_PORT}

# Install build dependencies: make, bash, curl, unzip, and libs for bun
RUN apk add --no-cache make bash curl unzip libgcc libstdc++

# Install bun using its official installer
RUN curl -fsSL https://bun.sh/install | bash
ENV PATH /root/.bun/bin:$PATH

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