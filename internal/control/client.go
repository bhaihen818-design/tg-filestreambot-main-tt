package control

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type RegisterRequest struct {
	MessageID int    `json:"message_id"`
	FileName  string `json:"file_name"`
	FileSize  int64  `json:"file_size"`
	MimeType  string `json:"mime_type"`
}

type RegisterResponse struct {
	AssetID     string `json:"asset_id"`
	ExpiresAt   string `json:"expires_at"`
	PlayerURL   string `json:"player_url"`
	DownloadURL string `json:"download_url"`
}

// Register sends only message metadata to GDPlayer. The request body never
// contains a Telegram file location, MTProto session, or any video bytes.
func Register(ctx context.Context, controlURL, sharedSecret string, request RegisterRequest, timeout time.Duration) (RegisterResponse, error) {
	if controlURL == "" || len(sharedSecret) < 32 {
		return RegisterResponse{}, errors.New("GDPlayer control integration is not configured")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return RegisterResponse{}, fmt.Errorf("encode control request: %w", err)
	}
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	mac := hmac.New(sha256.New, []byte(sharedSecret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)

	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodPost, controlURL, bytes.NewReader(body))
	if err != nil {
		return RegisterResponse{}, fmt.Errorf("create control request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-Control-Timestamp", timestamp)
	httpRequest.Header.Set("X-Control-Signature", hex.EncodeToString(mac.Sum(nil)))

	response, err := (&http.Client{Timeout: timeout}).Do(httpRequest)
	if err != nil {
		return RegisterResponse{}, fmt.Errorf("send control request: %w", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return RegisterResponse{}, fmt.Errorf("read control response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return RegisterResponse{}, fmt.Errorf("GDPlayer control endpoint rejected asset registration (%d)", response.StatusCode)
	}
	var result RegisterResponse
	if err := json.Unmarshal(payload, &result); err != nil {
		return RegisterResponse{}, fmt.Errorf("decode control response: %w", err)
	}
	if result.AssetID == "" || !strings.HasPrefix(result.PlayerURL, "http") {
		return RegisterResponse{}, errors.New("GDPlayer control endpoint returned an incomplete response")
	}
	return result, nil
}
