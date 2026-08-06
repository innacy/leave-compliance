package handler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/zinscky/log"
)

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
}

func getAccessToken(config map[string]string, log *log.Logger) (string, error) {
	url := config["greythrBaseURL"] + "/uas/v1/oauth2/client-token"
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	auth := config["Username"] + ":" + config["Password"]
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(auth)))

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
	if config["greythrBaseURL"] == "" || config["Username"] == "" || config["Password"] == "" {
		return "", errors.New("missing required configuration for GreyHR token generation")
	}

	token, err := getAccessToken(config, log)
	if err != nil {
		return "", err
	}
	vars["greythrToken"] = token

	return event, nil
}
