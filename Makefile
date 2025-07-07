# Go variables
BINARY_NAME=gogemini
BINARY_PATH=cmd/gogemini/$(BINARY_NAME)

# Frontend variables
FRONTEND_DIR=frontend

.PHONY: all build run clean dev

all: build

# Build the Go binary and the frontend
build:
	@echo "Building frontend..."
	@cd $(FRONTEND_DIR) && bun install && bun run build
	@echo "Building backend..."
	@go build -o $(BINARY_PATH) ./cmd/gogemini

# Run the application
run: build
	@echo "Starting application..."
	@./$(BINARY_PATH)

# Clean up build artifacts
clean:
	@echo "Cleaning up..."
	@rm -f $(BINARY_PATH)
	@rm -rf $(FRONTEND_DIR)/dist
	@rm -rf cmd/gogemini/dist

# Run frontend and backend in development mode
dev:
	@echo "Starting development servers..."
	@trap "kill 0" EXIT; \
	(cd $(FRONTEND_DIR) && bun run dev) & \
	(go run ./cmd/gogemini)