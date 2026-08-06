package main

import (
	"os"
	"testing"
)

func clearConfigEnv() {
	for _, key := range []string{
		"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD",
		"API_URL", "API_KEY", "API_KEY_HEADER",
	} {
		os.Setenv(key, "")
	}
}

func setValidConfigEnv() {
	os.Setenv("DB_HOST", "192.168.1.100")
	os.Setenv("DB_PORT", "1433")
	os.Setenv("DB_NAME", "etimetrackerlite1")
	os.Setenv("DB_USER", "sa")
	os.Setenv("DB_PASSWORD", "secret")
	os.Setenv("API_URL", "https://example.com/api/attendance")
	os.Setenv("API_KEY", "testkey123")
	os.Setenv("API_KEY_HEADER", "X-API-Key")
}

func TestLoadConfig_AllValid(t *testing.T) {
	clearConfigEnv()
	setValidConfigEnv()
	defer clearConfigEnv()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBHost != "192.168.1.100" {
		t.Errorf("DBHost = %q, want %q", cfg.DBHost, "192.168.1.100")
	}
	if cfg.DBPort != 1433 {
		t.Errorf("DBPort = %d, want %d", cfg.DBPort, 1433)
	}
	if cfg.DBName != "etimetrackerlite1" {
		t.Errorf("DBName = %q, want %q", cfg.DBName, "etimetrackerlite1")
	}
	if cfg.APIURL != "https://example.com/api/attendance" {
		t.Errorf("APIURL = %q, want %q", cfg.APIURL, "https://example.com/api/attendance")
	}
}

func TestLoadConfig_MissingRequired(t *testing.T) {
	clearConfigEnv()
	defer clearConfigEnv()

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing DB env vars, got nil")
	}
}

func TestLoadConfig_APIOptional(t *testing.T) {
	clearConfigEnv()
	os.Setenv("DB_HOST", "192.168.1.100")
	os.Setenv("DB_PORT", "1433")
	os.Setenv("DB_NAME", "etimetrackerlite1")
	os.Setenv("DB_USER", "sa")
	os.Setenv("DB_PASSWORD", "secret")
	defer clearConfigEnv()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIURL != "" {
		t.Errorf("APIURL = %q, want empty", cfg.APIURL)
	}
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	clearConfigEnv()
	setValidConfigEnv()
	os.Setenv("DB_PORT", "notanumber")
	defer clearConfigEnv()

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid port, got nil")
	}
}

func TestLoadConfig_InvalidURL(t *testing.T) {
	clearConfigEnv()
	setValidConfigEnv()
	os.Setenv("API_URL", "://bad-url")
	defer clearConfigEnv()

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}
