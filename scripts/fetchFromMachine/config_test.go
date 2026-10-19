package fetchfrommachine

import (
	"os"
	"testing"
)

func TestLoadConfig_Success(t *testing.T) {
	os.Setenv("ESSL_URL", "http://localhost:8089")
	os.Setenv("ESSL_USERNAME", "testuser")
	os.Setenv("ESSL_PASSWORD_ENCRYPTED", "encpass123")
	os.Setenv("ESSL_ENCRYPTION_KEY", "key123")
	os.Setenv("API_URL", "https://api.example.com/attendance")
	os.Setenv("API_KEY", "sk-test")
	defer func() {
		os.Unsetenv("ESSL_URL")
		os.Unsetenv("ESSL_USERNAME")
		os.Unsetenv("ESSL_PASSWORD_ENCRYPTED")
		os.Unsetenv("ESSL_ENCRYPTION_KEY")
		os.Unsetenv("API_URL")
		os.Unsetenv("API_KEY")
	}()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.ESSLURL != "http://localhost:8089" {
		t.Errorf("ESSLURL = %q, want %q", cfg.ESSLURL, "http://localhost:8089")
	}
	if cfg.DaysToFetch != 7 {
		t.Errorf("DaysToFetch = %d, want 7 (default)", cfg.DaysToFetch)
	}
}

func TestLoadConfig_MissingRequired(t *testing.T) {
	os.Clearenv()

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig() should fail when required vars missing")
	}
}
