package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/zinscky/log"
)

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

func getToken(config map[string]string) (string, error) {
	values := url.Values{}
	values.Set("client_id", config["clientId"])
	values.Set("client_secret", config["clientSecret"])
	values.Set("scope", "https://graph.microsoft.com/.default")
	values.Set("grant_type", "client_credentials")

	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", config["tenantId"])

	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", res.StatusCode, string(body))
	}

	var resp TokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("unmarshal token response: %w", err)
	}
	return resp.AccessToken, nil
}

func Execute(event string, config map[string]string, vars map[string]string, log *log.Logger) (string, error) {
	if config["clientId"] == "" || config["clientSecret"] == "" || config["tenantId"] == "" {
		return "", errors.New("missing required configuration for Microsoft token generation")
	}

	token, err := getToken(config)
	if err != nil {
		return "", err
	}
	vars["microsoftToken"] = token

	return event, nil
}
