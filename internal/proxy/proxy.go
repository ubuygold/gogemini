package proxy

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
)

type OpenAIProxy struct {
	geminiKeys   []string
	nextKeyIndex int
	mutex        sync.Mutex
	reverseProxy *httputil.ReverseProxy
	targetURL    *url.URL
	debug        bool
	logger       *slog.Logger
}

// newOpenAIProxyWithURL is the internal constructor that allows for custom target URLs, making it testable.
func newOpenAIProxyWithURL(geminiKeys []string, target string, debug bool, logger *slog.Logger) (*OpenAIProxy, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	proxy := &OpenAIProxy{
		geminiKeys: geminiKeys,
		targetURL:  targetURL,
		debug:      debug,
		logger:     logger.With("component", "proxy"),
	}

	proxy.reverseProxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = proxy.targetURL.Scheme
			req.URL.Host = proxy.targetURL.Host
			req.Host = proxy.targetURL.Host

			// Manually construct the full path to avoid issues with url.ResolveReference.
			// The original path from the client (after StripPrefix in main.go) is e.g., "/v1/chat/completions".
			// We strip "/v1" and prepend the correct API path.
			trimmedPath := strings.TrimPrefix(req.URL.Path, "/v1")
			req.URL.Path = "/v1beta/openai" + trimmedPath

			// Use the next key in a round-robin fashion
			proxy.mutex.Lock()
			key := proxy.geminiKeys[proxy.nextKeyIndex]
			proxy.nextKeyIndex = (proxy.nextKeyIndex + 1) % len(proxy.geminiKeys)
			proxy.mutex.Unlock()

			if proxy.debug {
				proxy.logger.Debug("Proxying request", "path", req.URL.Path, "key_suffix", key[len(key)-4:])
			}

			// Set the Authorization header for the upstream request.
			// The compatible endpoint expects a bearer token.
			req.Header.Set("Authorization", "Bearer "+key)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, context.Canceled) || errors.Is(err, http.ErrAbortHandler) {
				proxy.logger.Warn("Client disconnected", "error", err)
				return
			}
			proxy.logger.Error("Proxy error", "error", err)
			http.Error(w, "Proxy Error", http.StatusBadGateway)
		},
	}

	return proxy, nil
}

// NewOpenAIProxy creates a new OpenAIProxy with the default Google API target.
func NewOpenAIProxy(geminiKeys []string, debug bool, logger *slog.Logger) (*OpenAIProxy, error) {
	return newOpenAIProxyWithURL(geminiKeys, "https://generativelanguage.googleapis.com", debug, logger)
}

func (p *OpenAIProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.reverseProxy.ServeHTTP(w, r)
}
