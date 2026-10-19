package fetchfrommachine

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	ESSLURL               string
	ESSLUsername          string
	ESSLPasswordEncrypted string
	ESSLEncryptionKey     string
	APIURL                string
	APIKey                string
	APIKeyHeader          string
	DaysToFetch           int
}

func LoadConfig() (*Config, error) {
	godotenv.Load()

	required := []string{
		"ESSL_URL",
		"ESSL_USERNAME",
		"ESSL_PASSWORD_ENCRYPTED",
		"ESSL_ENCRYPTION_KEY",
		"API_URL",
		"API_KEY",
	}

	var missing []string
	for _, key := range required {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missing)
	}

	daysToFetch := 7
	if val := os.Getenv("DAYS_TO_FETCH"); val != "" {
		if d, err := strconv.Atoi(val); err == nil && d > 0 {
			daysToFetch = d
		}
	}

	apiKeyHeader := os.Getenv("API_KEY_HEADER")
	if apiKeyHeader == "" {
		apiKeyHeader = "X-API-Key"
	}

	return &Config{
		ESSLURL:               os.Getenv("ESSL_URL"),
		ESSLUsername:          os.Getenv("ESSL_USERNAME"),
		ESSLPasswordEncrypted: os.Getenv("ESSL_PASSWORD_ENCRYPTED"),
		ESSLEncryptionKey:     os.Getenv("ESSL_ENCRYPTION_KEY"),
		APIURL:                os.Getenv("API_URL"),
		APIKey:                os.Getenv("API_KEY"),
		APIKeyHeader:          apiKeyHeader,
		DaysToFetch:           daysToFetch,
	}, nil
}
