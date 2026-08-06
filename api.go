package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type FullAttendance struct {
	Data []DayAttendance `json:"data"`
}

func PostAttendance(cfg *Config, data []DayAttendance) (int, error) {
	result := FullAttendance{Data: data}
	body, err := json.Marshal(result)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal attendance data: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest(http.MethodPost, cfg.APIURL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(cfg.APIKeyHeader, cfg.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	return resp.StatusCode, nil
}
