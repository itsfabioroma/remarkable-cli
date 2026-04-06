package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fabioroma/remarkable-cli/pkg/model"
	"github.com/google/uuid"
)

// cloud API endpoints (from community reverse-engineering of rmapi/rmfakecloud)
const (
	authHost     = "https://webapp-prod.cloud.remarkable.engineering"
	deviceNewURL = authHost + "/token/json/2/device/new"
	userNewURL   = authHost + "/token/json/2/user/new"
)

// TokenStore persists device and user tokens
type TokenStore struct {
	configDir string
}

// Tokens holds the authentication tokens
type Tokens struct {
	DeviceToken string `json:"deviceToken"`
	UserToken   string `json:"userToken"`
	DeviceID    string `json:"deviceId"`
}

// NewTokenStore creates a store at ~/.config/remarkable-cli/
func NewTokenStore() *TokenStore {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "remarkable-cli")
	os.MkdirAll(dir, 0700)
	return &TokenStore{configDir: dir}
}

func (s *TokenStore) path() string {
	return filepath.Join(s.configDir, "tokens.json")
}

// Load reads stored tokens from disk
func (s *TokenStore) Load() (*Tokens, error) {
	data, err := os.ReadFile(s.path())
	if err != nil {
		return nil, err
	}
	var t Tokens
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// Save writes tokens to disk
func (s *TokenStore) Save(t *Tokens) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(), data, 0600)
}

// RegisterDevice exchanges a one-time code for a device token
// User gets the code from https://my.remarkable.com/device/browser/connect
func RegisterDevice(code string) (*Tokens, error) {
	deviceID := uuid.New().String()

	body := map[string]string{
		"code":       code,
		"deviceDesc": "desktop-linux",
		"deviceID":   deviceID,
	}

	jsonBody, _ := json.Marshal(body)

	resp, err := http.Post(deviceNewURL, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, model.NewCLIError(model.ErrTransportUnavailable, "cloud",
			fmt.Sprintf("cannot reach auth server: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, model.NewCLIError(model.ErrAuthRequired, "cloud",
			fmt.Sprintf("device registration failed (%d): %s", resp.StatusCode, string(respBody)))
	}

	// response body IS the device token (plain text, not JSON)
	tokenBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	tokens := &Tokens{
		DeviceToken: string(tokenBytes),
		DeviceID:    deviceID,
	}

	// immediately get a user token
	if err := refreshUserToken(tokens); err != nil {
		return tokens, fmt.Errorf("device registered but user token failed: %w", err)
	}

	return tokens, nil
}

// refreshUserToken exchanges a device token for a user token
func refreshUserToken(tokens *Tokens) error {
	client := &http.Client{Timeout: 10 * time.Second}

	req, _ := http.NewRequest("POST", userNewURL, nil)
	req.Header.Set("Authorization", "Bearer "+tokens.DeviceToken)

	resp, err := client.Do(req)
	if err != nil {
		return model.NewCLIError(model.ErrTransportUnavailable, "cloud",
			fmt.Sprintf("cannot reach auth server: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return model.NewCLIError(model.ErrAuthExpired, "cloud",
			fmt.Sprintf("user token refresh failed (%d)", resp.StatusCode))
	}

	tokenBytes, _ := io.ReadAll(resp.Body)
	tokens.UserToken = string(tokenBytes)
	return nil
}

// EnsureAuth loads tokens or prompts for registration
// Returns tokens ready for API calls
func EnsureAuth(store *TokenStore) (*Tokens, error) {
	// try loading existing tokens
	tokens, err := store.Load()
	if err == nil && tokens.DeviceToken != "" {
		// try refreshing user token
		if err := refreshUserToken(tokens); err == nil {
			store.Save(tokens)
			return tokens, nil
		}
	}

	return nil, model.NewCLIError(model.ErrAuthRequired, "cloud",
		"not authenticated. Run 'remarkable auth' to connect to reMarkable Cloud")
}
