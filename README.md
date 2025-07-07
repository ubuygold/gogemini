# Go Gemini Balancer

Go Gemini Balancer is a sophisticated reverse proxy designed for high availability and efficient management of Google Gemini and OpenAI API keys. It provides load balancing, automatic failover, and a full-featured web UI for administration.

 <!-- It's recommended to add a real screenshot here -->

## Key Features

- **Multi-Key Load Balancing**: Distributes API requests across multiple Gemini and OpenAI keys to avoid rate limits and optimize usage.
- **Resilient & Self-Healing**: Automatically detects failing keys, disables them, and periodically attempts to revive them.
- **Unified API Endpoint**: Provides a single, consistent endpoint for both Gemini and OpenAI compatible requests.
- **Web Admin UI**: A full-featured dashboard to manage, monitor, test, and configure API keys without restarting the server.
- **Persistent Storage**: Uses a database (SQLite, PostgreSQL, MySQL) to store key configurations and statistics.
- **Client-Side Keys**: Supports client-specific API keys for authenticated access to the proxy.
- **Containerized Deployment**: Fully containerized with Docker and Docker Compose for easy and reproducible deployments.
- **Flexible Configuration**: Configure the application via `config.yaml` or override settings with environment variables.

## Getting Started

The easiest way to run Go Gemini Balancer is with Docker Compose.

### Prerequisites

- Docker and Docker Compose installed.
- Git (for cloning the repository).

### 1. Clone the Repository

```bash
git clone https://github.com/your-username/gogemini.git
cd gogemini
```

### 2. Configure

The project uses a `.env` file for configuration with Docker Compose. An example is provided in `.env.example`.

```bash
# Create your own .env file from the example
cp .env.example .env
```

Now, edit the `.env` file to set your desired configuration. At a minimum, you should set a secure admin password.

```dotenv
# .env

# Gogemini App Configuration
GOGEMINI_PORT=8081
GOGEMINI_ADMIN_PASSWORD=your-super-secure-password
GOGEMINI_DEBUG=false

# PostgreSQL Database Configuration
POSTGRES_USER=gogemini
POSTGRES_PASSWORD=gogemini
POSTGRES_DB=gogemini
POSTGRES_PORT=5432

# ... other variables
```

### 3. Run with Docker Compose

```bash
docker-compose up --build -d
```

The application will be available at `http://localhost:8081`.

## Usage

### Admin Panel

Navigate to `http://localhost:8081` in your browser. Log in with the admin password you set in your `.env` file (`GOGEMINI_ADMIN_PASSWORD`).

From the admin panel, you can:
- Add, delete, and manage your Gemini and OpenAI API keys.
- View usage statistics for each key.
- Manually test the validity of keys.
- Enable or disable keys.
- Manage client API keys for proxy access.

### API Endpoints

- **Gemini Proxy**: `http://localhost:8081/gemini`
- **OpenAI Proxy**: `http://localhost:8081/openai`

To use these endpoints, you must provide a client API key (which you can create in the admin panel) in the `Authorization` header.

```bash
curl -X POST http://localhost:8081/gemini/v1beta/models/gemini-pro:generateContent \
-H "Content-Type: application/json" \
-H "Authorization: Bearer YOUR_CLIENT_API_KEY" \
-d '{
  "contents": [{"parts":[{"text": "Write a story about a magic backpack."}]}]
}'
```

## Manual Installation (Without Docker)

If you prefer to run the application directly:

### 1. Configuration

Edit `config.yaml` to set your configuration. You'll need to provide your database details and at least one Gemini or OpenAI key to get started.

```yaml
port: 8081
debug: true
database:
  type: "sqlite" # or "postgres" or "mysql"
  dsn: "gemini.db" # or your DSN for postgres/mysql
admin:
  password: "your-secure-password"
# Add initial keys if desired
# gemini_keys:
#   - "YOUR_GEMINI_API_KEY_1"
```

### 2. Build and Run

```bash
# Build the frontend
cd frontend
bun install
bun run build
cd ..

# Run the Go server
go run ./cmd/gogemini
```

## Configuration Details

The application can be configured via `config.yaml` and overridden by environment variables.

| `config.yaml` Key         | Environment Variable          | Description                               | Default      |
| ------------------------- | ----------------------------- | ----------------------------------------- | ------------ |
| `port`                    | `GOGEMINI_PORT`               | The port the server listens on.           | `8081`       |
| `debug`                   | `GOGEMINI_DEBUG`              | Enable or disable debug logging.          | `false`      |
| `admin.password`          | `GOGEMINI_ADMIN_PASSWORD`     | Password for the admin UI.                | `dev-password` |
| `database.type`           | `GOGEMINI_DATABASE_TYPE`      | Database type (`sqlite`, `postgres`, `mysql`). | `sqlite`     |
| `database.dsn`            | `GOGEMINI_DATABASE_DSN`       | Database connection string.               | `gemini.db`  |
| `proxy.disable_key_threshold` | -                             | Consecutive failures to disable a key.    | `3`          |
| `scheduler.key_revival_interval` | -                          | How often to re-test disabled keys.       | `10m`        |

## Contributing

Contributions are welcome! Please feel free to submit a pull request or open an issue.
