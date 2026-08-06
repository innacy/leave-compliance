package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostAttendance_Success(t *testing.T) {
	var receivedBody []DayAttendance
	var receivedHeader string
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedHeader = r.Header.Get("X-API-Key")

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	cfg := &Config{
		APIURL:       server.URL,
		APIKey:       "testkey123",
		APIKeyHeader: "X-API-Key",
	}

	data := []DayAttendance{
		{Date: "2026-07-20", Present: []string{"EMP001", "EMP002"}},
	}

	statusCode, err := PostAttendance(cfg, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("status code = %d, want %d", statusCode, http.StatusOK)
	}
	if receivedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", receivedContentType, "application/json")
	}
	if receivedHeader != "testkey123" {
		t.Errorf("API key header = %q, want %q", receivedHeader, "testkey123")
	}
	if len(receivedBody) != 1 {
		t.Fatalf("received %d items, want 1", len(receivedBody))
	}
	if receivedBody[0].Date != "2026-07-20" {
		t.Errorf("date = %q, want %q", receivedBody[0].Date, "2026-07-20")
	}
}

func TestPostAttendance_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	cfg := &Config{
		APIURL:       server.URL,
		APIKey:       "testkey123",
		APIKeyHeader: "X-API-Key",
	}

	statusCode, err := PostAttendance(cfg, []DayAttendance{})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if statusCode != http.StatusInternalServerError {
		t.Errorf("status code = %d, want %d", statusCode, http.StatusInternalServerError)
	}
}

func TestPostAttendance_NetworkError(t *testing.T) {
	cfg := &Config{
		APIURL:       "http://localhost:1",
		APIKey:       "testkey123",
		APIKeyHeader: "X-API-Key",
	}

	statusCode, err := PostAttendance(cfg, []DayAttendance{})
	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
	if statusCode != 0 {
		t.Errorf("status code = %d, want 0", statusCode)
	}
}
