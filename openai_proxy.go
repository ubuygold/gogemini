package main

import (
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
}

func NewOpenAIProxy(geminiKeys []string) (*OpenAIProxy, error) {
	targetURL, err := url.Parse("https://generativelanguage.googleapis.com")
	if err != nil {
		return nil, err
	}

	proxy := &OpenAIProxy{
		geminiKeys: geminiKeys,
		targetURL:  targetURL,
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

			// Set the Authorization header for the upstream request.
			// The compatible endpoint expects a bearer token.
			req.Header.Set("Authorization", "Bearer "+key)
		},
	}

	return proxy, nil
}

func (p *OpenAIProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.reverseProxy.ServeHTTP(w, r)
}
