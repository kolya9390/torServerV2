package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	torrentsPath             = "/api/v1/torrents"
	torrentUploadPath        = "/api/v1/torrent/upload"
	settingsPath             = "/api/v1/settings"
	authUsersPath            = "/api/v1/auth/users"
	shutdownTokenPath        = "/api/v1/config/shutdown-token" // #nosec G101 -- endpoint path, not a credential.
	shutdownTokenGenerateURL = shutdownTokenPath + "/generate"
)

type torrentRequest struct {
	Action   string `json:"action"`
	Link     string `json:"link,omitempty"`
	Hash     string `json:"hash,omitempty"`
	Title    string `json:"title,omitempty"`
	Poster   string `json:"poster,omitempty"`
	Category string `json:"category,omitempty"`
	Data     string `json:"data,omitempty"`
	Save     bool   `json:"save_to_db,omitempty"`
}

type settingsRequest struct {
	Action string        `json:"action"`
	Sets   SettingsPatch `json:"sets,omitempty"`
}

type addUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"` // #nosec G117 -- caller-provided password sent to the authenticated API.
}

type setShutdownTokenRequest struct {
	Token string `json:"token"` // #nosec G117 -- caller-provided token sent to the authenticated API.
}

// Version returns the server HTTP API version contract.
func (client *Client) Version(ctx context.Context) (Version, error) {
	var response Version
	if err := client.doJSON(ctx, http.MethodGet, versionPath, nil, &response, nil); err != nil {
		return Version{}, classifyCompatibilityError(err)
	}

	if err := normalizeAndValidateVersion(&response); err != nil {
		return Version{}, err
	}

	return response, nil
}

// Readiness returns server component readiness.
func (client *Client) Readiness(ctx context.Context) (Readiness, error) {
	var response Readiness
	err := client.doJSON(ctx, http.MethodGet, "/readyz", nil, &response, nil)

	return response, err
}

// ListTorrents returns all server torrent statuses.
func (client *Client) ListTorrents(ctx context.Context) ([]Torrent, error) {
	var response []Torrent
	err := client.doJSON(
		ctx,
		http.MethodPost,
		torrentsPath,
		torrentRequest{Action: "list"},
		&response,
		nil,
	)

	return response, err
}

// GetTorrent returns one torrent status by canonical hash.
func (client *Client) GetTorrent(ctx context.Context, hash string) (Torrent, error) {
	var response Torrent
	err := client.doJSON(
		ctx,
		http.MethodPost,
		torrentsPath,
		torrentRequest{Action: "get", Hash: hash},
		&response,
		nil,
	)

	return response, err
}

// AddTorrent adds a magnet, hash, or link to the server.
func (client *Client) AddTorrent(ctx context.Context, input AddTorrentRequest) (Torrent, error) {
	var response Torrent
	err := client.doJSON(ctx, http.MethodPost, torrentsPath, torrentRequest{
		Action:   "add",
		Link:     input.Link,
		Title:    input.Title,
		Poster:   input.Poster,
		Category: input.Category,
		Data:     input.Data,
		Save:     input.Save,
	}, &response, nil)

	return response, err
}

// UploadTorrent uploads one local torrent file to the server.
func (client *Client) UploadTorrent(ctx context.Context, input UploadTorrentRequest) (Torrent, error) {
	fields := map[string]string{
		"title":    input.Title,
		"poster":   input.Poster,
		"category": input.Category,
		"data":     input.Data,
	}
	if input.Save {
		fields["save"] = strconv.FormatBool(true)
	}

	var response Torrent
	err := client.doMultipartFile(ctx, torrentUploadPath, input.FilePath, fields, &response)

	return response, err
}

// RemoveTorrent removes a torrent from the server list.
func (client *Client) RemoveTorrent(ctx context.Context, hash string) error {
	return client.torrentHashAction(ctx, "rem", hash)
}

// DropTorrent removes a torrent and its active runtime resources when allowed.
func (client *Client) DropTorrent(ctx context.Context, hash string) error {
	return client.torrentHashAction(ctx, "drop", hash)
}

// WipeTorrents removes every torrent known to the server.
func (client *Client) WipeTorrents(ctx context.Context) error {
	return client.doJSON(
		ctx,
		http.MethodPost,
		torrentsPath,
		torrentRequest{Action: "wipe"},
		nil,
		nil,
	)
}

func (client *Client) torrentHashAction(ctx context.Context, action, hash string) error {
	return client.doJSON(
		ctx,
		http.MethodPost,
		torrentsPath,
		torrentRequest{Action: action, Hash: hash},
		nil,
		nil,
	)
}

// GetSettings returns the current dynamic server settings document.
func (client *Client) GetSettings(ctx context.Context) (Settings, error) {
	var response Settings
	err := client.doJSON(
		ctx,
		http.MethodPost,
		settingsPath,
		settingsRequest{Action: "get"},
		&response,
		nil,
	)

	return response, err
}

// SetSettings applies a validated partial settings update.
func (client *Client) SetSettings(ctx context.Context, patch SettingsPatch) error {
	return client.doJSON(
		ctx,
		http.MethodPost,
		settingsPath,
		settingsRequest{Action: "set", Sets: patch},
		nil,
		nil,
	)
}

// ResetSettings restores server defaults.
func (client *Client) ResetSettings(ctx context.Context) error {
	return client.doJSON(
		ctx,
		http.MethodPost,
		settingsPath,
		settingsRequest{Action: "def"},
		nil,
		nil,
	)
}

// ListUsers returns usernames and creation timestamps without password data.
func (client *Client) ListUsers(ctx context.Context) (Users, error) {
	var response Users
	err := client.doJSON(ctx, http.MethodGet, authUsersPath, nil, &response, nil)

	return response, err
}

// AddUser creates one server authentication user.
func (client *Client) AddUser(ctx context.Context, username, password string) error {
	return client.doJSON(
		ctx,
		http.MethodPost,
		authUsersPath,
		addUserRequest{Username: username, Password: password},
		nil,
		nil,
	)
}

// RemoveUser removes one server authentication user.
func (client *Client) RemoveUser(ctx context.Context, username string) error {
	return client.doJSON(
		ctx,
		http.MethodDelete,
		authUsersPath+"/"+url.PathEscape(username),
		nil,
		nil,
		nil,
	)
}

// ShutdownTokenStatus reports whether a shutdown token is configured.
func (client *Client) ShutdownTokenStatus(ctx context.Context) (ShutdownTokenStatus, error) {
	var response ShutdownTokenStatus
	err := client.doJSON(ctx, http.MethodGet, shutdownTokenPath, nil, &response, nil)

	return response, err
}

// GenerateShutdownToken generates and stores a new shutdown token.
func (client *Client) GenerateShutdownToken(ctx context.Context) (GeneratedShutdownToken, error) {
	var response GeneratedShutdownToken
	err := client.doJSON(ctx, http.MethodPost, shutdownTokenGenerateURL, nil, &response, nil)

	return response, err
}

// SetShutdownToken stores a caller-provided shutdown token.
func (client *Client) SetShutdownToken(ctx context.Context, token string) error {
	return client.doJSON(
		ctx,
		http.MethodPost,
		shutdownTokenPath,
		setShutdownTokenRequest{Token: token},
		nil,
		nil,
	)
}

// Shutdown requests graceful daemon shutdown.
func (client *Client) Shutdown(ctx context.Context, input ShutdownRequest) error {
	endpoint := "/api/v1/shutdown"
	if reason := strings.TrimSpace(input.Reason); reason != "" {
		endpoint += "/" + url.PathEscape(reason)
	}

	headers := make(http.Header)
	if strings.EqualFold(strings.TrimSpace(input.Mode), "public") && strings.TrimSpace(input.Token) != "" {
		headers.Set("X-TS-Shutdown-Token", strings.TrimSpace(input.Token))
	}

	return client.doJSON(ctx, http.MethodPost, endpoint, nil, nil, headers)
}
