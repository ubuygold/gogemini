package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

func TestGetNextKey(t *testing.T) {
	config := &Config{
		GeminiKeys: []string{"key1", "key2", "key3"},
	}
	balancer := NewBalancer(config, nil)

	expectedKeys := []string{"key1", "key2", "key3", "key1"}
	for _, expectedKey := range expectedKeys {
		actualKey := balancer.getNextKey()
		if actualKey != expectedKey {
			t.Errorf("Expected key %s, but got %s", expectedKey, actualKey)
		}
	}
}

func TestNewBalancer(t *testing.T) {
	config := &Config{
		ClientKeys: []string{"client1", "client2"},
	}
	balancer := NewBalancer(config, nil)

	expectedMap := map[string]bool{
		"client1": true,
		"client2": true,
	}

	if !reflect.DeepEqual(balancer.authorizedKeys, expectedMap) {
		t.Errorf("Expected authorizedKeys map to be %v, but got %v", expectedMap, balancer.authorizedKeys)
	}
}

func TestBalancer_ServeHTTP(t *testing.T) {
	// --- Setup ---
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that the proxy is forwarding with its own key, not the client's.
		if r.Header.Get("x-goog-api-key") != "gemini-pool-key" {
			t.Errorf("Expected x-goog-api-key to be 'gemini-pool-key', got '%s'", r.Header.Get("x-goog-api-key"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	backendURL, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}

	config := &Config{
		ClientKeys: []string{"valid-client-key"},
		GeminiKeys: []string{"gemini-pool-key"},
		Debug:      false, // Keep tests quiet
	}
	balancer := NewBalancer(config, backendURL)

	// --- Test Cases ---
	t.Run("Authorized request", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/", nil)
		req.Header.Set("x-goog-api-key", "valid-client-key")
		rr := httptest.NewRecorder()

		balancer.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
	})

	t.Run("Unauthorized with invalid key", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/", nil)
		req.Header.Set("x-goog-api-key", "invalid-client-key")
		rr := httptest.NewRecorder()

		balancer.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusUnauthorized {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusUnauthorized)
		}
	})

	t.Run("Unauthorized with missing key", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/", nil)
		// No key set
		rr := httptest.NewRecorder()

		balancer.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusUnauthorized {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusUnauthorized)
		}
	})
}
