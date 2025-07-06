# Go Gemini Balancer

A simple reverse proxy to load balance requests to the Gemini API using multiple API keys.

## How it works

This tool acts as a reverse proxy that listens for incoming HTTP requests. When a request is received, it forwards it to the official Gemini API endpoint (`https://generativelanguage.googleapis.com`).

It uses a round-robin strategy to select a Gemini API key from a list you provide in the `config.yaml` file. This allows you to distribute your API usage across multiple keys, avoiding rate limits and managing costs.

The proxy is designed to be compatible with clients that use the OpenAI API format, but it forwards requests to the native Gemini API endpoint. You may need to adjust your client-side code to match the expected request/response format of the Gemini API.

## Getting Started

### 1. Configuration

Edit the `config.yaml` file to add your Gemini API keys:

```yaml
gemini_keys:
  - "YOUR_GEMINI_API_KEY_1"
  - "YOUR_GEMINI_API_KEY_2"
  # Add more keys as needed

port: 8080
```

### 2. Run the server

```bash
go mod tidy
go run main.go
```

The server will start on port `8080` (or the port you specified in the config).

### 3. Make a request

You can now send API requests to `http://localhost:8080`. The proxy will add the `x-goog-api-key` header and forward the request to Google.

Example using `curl`:

```bash
curl -X POST http://localhost:8080/v1beta/models/gemini-pro:generateContent \
-H "Content-Type: application/json" \
-d '{
  "contents": [{
    "parts":[{
      "text": "Write a story about a magic backpack."
    }]
  }]
}'
```
