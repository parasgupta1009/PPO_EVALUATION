package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PPO_EVALUATION/services"
)

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	store := services.NewKVStore(ctx, 10*time.Second)
	handler := NewStoreHandler(store)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	return httptest.NewServer(mux)
}

func TestE2E_SetAndGet_HappyPath(t *testing.T) {
	t.Parallel()
	server := setupTestServer(t)
	defer server.Close()

	// SET a key
	body := `{"key":"user:1","value":"Alice","ttl_seconds":60}`
	resp, err := http.Post(server.URL+"/keys", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("SET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("SET status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var setResp map[string]string
	json.NewDecoder(resp.Body).Decode(&setResp)
	if setResp["message"] != "key set successfully" {
		t.Fatalf("SET message = %q, want %q", setResp["message"], "key set successfully")
	}
	if setResp["key"] != "user:1" {
		t.Fatalf("SET key = %q, want %q", setResp["key"], "user:1")
	}

	// GET the key
	resp, err = http.Get(server.URL + "/keys/user:1")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var getResp GetResponse
	if err := json.NewDecoder(resp.Body).Decode(&getResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if getResp.Key != "user:1" || getResp.Value != "Alice" {
		t.Fatalf("GET response = %+v, want key=user:1, value=Alice", getResp)
	}
}

func TestE2E_Delete_HappyPath(t *testing.T) {
	t.Parallel()
	server := setupTestServer(t)
	defer server.Close()

	// SET a key
	body := `{"key":"temp","value":"data","ttl_seconds":60}`
	resp, err := http.Post(server.URL+"/keys", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("SET request failed: %v", err)
	}
	resp.Body.Close()

	// DELETE the key
	req, _ := http.NewRequest(http.MethodDelete, server.URL+"/keys/temp", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var delResp map[string]string
	json.NewDecoder(resp.Body).Decode(&delResp)
	if delResp["message"] != "deleted successfully" {
		t.Fatalf("DELETE message = %q, want %q", delResp["message"], "deleted successfully")
	}

	// GET should return 404
	resp2, err := http.Get(server.URL + "/keys/temp")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	resp2.Body.Close()

	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("GET after DELETE status = %d, want %d", resp2.StatusCode, http.StatusNotFound)
	}
}

func TestE2E_Delete_KeyNotFound(t *testing.T) {
	t.Parallel()
	server := setupTestServer(t)
	defer server.Close()

	req, _ := http.NewRequest(http.MethodDelete, server.URL+"/keys/nonexistent", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	var errResp ErrorResponse
	json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp.Error != "key not found" {
		t.Fatalf("error = %q, want %q", errResp.Error, "key not found")
	}
}

func TestE2E_Delete_KeyExpired(t *testing.T) {
	t.Parallel()
	server := setupTestServer(t)
	defer server.Close()

	// SET with 1-second TTL
	body := `{"key":"expiring","value":"val","ttl_seconds":1}`
	resp, err := http.Post(server.URL+"/keys", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("SET request failed: %v", err)
	}
	resp.Body.Close()

	time.Sleep(1500 * time.Millisecond)

	// DELETE expired key → 410
	req, _ := http.NewRequest(http.MethodDelete, server.URL+"/keys/expiring", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusGone {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusGone)
	}

	var errResp ErrorResponse
	json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp.Error != "key expired" {
		t.Fatalf("error = %q, want %q", errResp.Error, "key expired")
	}
}

func TestE2E_Get_KeyNotFound(t *testing.T) {
	t.Parallel()
	server := setupTestServer(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/keys/nonexistent")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	var errResp ErrorResponse
	json.NewDecoder(resp.Body).Decode(&errResp)

	if errResp.Error != "key not found" {
		t.Fatalf("error = %q, want %q", errResp.Error, "key not found")
	}
}

func TestE2E_Get_KeyExpired(t *testing.T) {
	t.Parallel()
	server := setupTestServer(t)
	defer server.Close()

	// SET with 1-second TTL
	body := `{"key":"short","value":"lived","ttl_seconds":1}`
	resp, err := http.Post(server.URL+"/keys", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("SET request failed: %v", err)
	}
	resp.Body.Close()

	// Wait for expiry
	time.Sleep(1500 * time.Millisecond)

	// GET should return 410 Gone
	resp, err = http.Get(server.URL + "/keys/short")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusGone {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusGone)
	}

	var errResp ErrorResponse
	json.NewDecoder(resp.Body).Decode(&errResp)

	if errResp.Error != "key expired" {
		t.Fatalf("error = %q, want %q", errResp.Error, "key expired")
	}
}

func TestE2E_Set_InvalidBody(t *testing.T) {
	t.Parallel()
	server := setupTestServer(t)
	defer server.Close()

	resp, err := http.Post(server.URL+"/keys", "application/json", bytes.NewBufferString("not json"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestE2E_Set_MissingKey(t *testing.T) {
	t.Parallel()
	server := setupTestServer(t)
	defer server.Close()

	body := `{"value":"data","ttl_seconds":60}`
	resp, err := http.Post(server.URL+"/keys", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	json.NewDecoder(resp.Body).Decode(&errResp)

	if errResp.Error != "key is required" {
		t.Fatalf("error = %q, want %q", errResp.Error, "key is required")
	}
}

func TestE2E_Set_InvalidTTL(t *testing.T) {
	t.Parallel()
	server := setupTestServer(t)
	defer server.Close()

	body := `{"key":"k","value":"v","ttl_seconds":0}`
	resp, err := http.Post(server.URL+"/keys", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	json.NewDecoder(resp.Body).Decode(&errResp)

	if errResp.Error != "ttl_seconds must be positive" {
		t.Fatalf("error = %q, want %q", errResp.Error, "ttl_seconds must be positive")
	}
}

func TestE2E_Set_OverwriteKey(t *testing.T) {
	t.Parallel()
	server := setupTestServer(t)
	defer server.Close()

	// SET first value
	body := `{"key":"dup","value":"first","ttl_seconds":60}`
	resp, _ := http.Post(server.URL+"/keys", "application/json", bytes.NewBufferString(body))
	resp.Body.Close()

	// SET second value (overwrite)
	body = `{"key":"dup","value":"second","ttl_seconds":60}`
	resp, _ = http.Post(server.URL+"/keys", "application/json", bytes.NewBufferString(body))
	resp.Body.Close()

	// GET should return latest value
	resp, err := http.Get(server.URL + "/keys/dup")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	var getResp GetResponse
	json.NewDecoder(resp.Body).Decode(&getResp)

	if getResp.Value != "second" {
		t.Fatalf("value = %q, want %q", getResp.Value, "second")
	}
}
