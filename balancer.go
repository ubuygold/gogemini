package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

// Balancer holds the state of our load balancer.
type Balancer struct {
	config         *Config
	mutex          sync.Mutex
	nextKey        int
	proxy          *httputil.ReverseProxy
	authorizedKeys map[string]bool
}

// NewBalancer creates a new Balancer.
func NewBalancer(config *Config, targetURL *url.URL) *Balancer {
	// For efficient lookup, convert the list of authorized keys into a map
	authorizedKeys := make(map[string]bool)
	for _, key := range config.ClientKeys {
		authorizedKeys[key] = true
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	balancer := &Balancer{
		config:         config,
		proxy:          proxy,
		authorizedKeys: authorizedKeys,
	}

	// Attach a director to modify the request before forwarding
	proxy.Director = func(req *http.Request) {
		// This is the key part: we are REPLACING the client's key with one from our pool.
		req.Header.Set("x-goog-api-key", balancer.getNextKey())

		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
		req.Host = targetURL.Host
		req.Header.Del("Authorization") // Not needed by Gemini

		if balancer.config.Debug {
			dump, err := httputil.DumpRequestOut(req, true)
			if err != nil {
				log.Println("Error dumping outgoing request:", err)
			} else {
				log.Printf(">>> Forwarding request to Gemini (with proxy's key):\n%s", string(dump))
			}
		}
	}

	// Attach a ModifyResponse to log the response from the target
	proxy.ModifyResponse = func(resp *http.Response) error {
		if balancer.config.Debug {
			dump, err := httputil.DumpResponse(resp, true)
			if err != nil {
				log.Println("Error dumping response:", err)
			} else {
				log.Printf("<<< Received response from Gemini:\n%s", string(dump))
			}
		}
		return nil
	}

	// Attach an error handler
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("Proxy error: %v", err)
		http.Error(w, "Proxy Error", http.StatusBadGateway)
	}

	return balancer
}

// getNextKey gets the next key in a round-robin fashion.
func (b *Balancer) getNextKey() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	key := b.config.GeminiKeys[b.nextKey]
	b.nextKey = (b.nextKey + 1) % len(b.config.GeminiKeys)
	return key
}

// ServeHTTP is the handler for all incoming requests.
func (b *Balancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// --- Authorization Check ---
	clientAPIKey := r.Header.Get("x-goog-api-key")
	if !b.authorizedKeys[clientAPIKey] {
		log.Printf("Unauthorized access attempt with key: %s", clientAPIKey)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if b.config.Debug {
		// To log the body, we need to read it and then replace it.
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Error reading request body: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		r.Body.Close() //  must close
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		dump, err := httputil.DumpRequest(r, false) // Dump without body first
		if err != nil {
			log.Println("Error dumping incoming request:", err)
		} else {
			log.Printf("<<< Received authorized request from client:\n%s%s", string(dump), string(bodyBytes))
		}

		// After logging, we must replace the body again for the proxy to read it.
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	b.proxy.ServeHTTP(w, r)
}
