package apiclient

import (
	"encoding/json"
	"fmt"
)

// Version describes the HTTP API compatibility surface exposed by the server.
type Version struct {
	Product            string   `json:"product"`
	ApplicationVersion string   `json:"application_version"`
	Current            string   `json:"current"`
	Capabilities       []string `json:"capabilities,omitempty"`
	Deprecated         []string `json:"deprecated,omitempty"`
	Deprecation        string   `json:"deprecation,omitempty"`
	Sunset             string   `json:"sunset,omitempty"`
	LegacyContract     bool     `json:"legacy_contract,omitempty"`
}

// Readiness describes whether HTTP and torrent runtime components are ready.
type Readiness struct {
	Status  string `json:"status"`
	HTTP    bool   `json:"http"`
	Torrent bool   `json:"torrent"`
}

// Torrent is the management-client read model for a torrent status response.
type Torrent struct {
	Title               string        `json:"title"`
	Category            string        `json:"category"`
	Poster              string        `json:"poster"`
	Data                string        `json:"data,omitempty"`
	Timestamp           int64         `json:"timestamp"`
	Name                string        `json:"name,omitempty"`
	Hash                string        `json:"hash,omitempty"`
	TorrsHash           string        `json:"torrs_hash,omitempty"`
	Stat                int           `json:"stat"`
	StatString          string        `json:"stat_string"`
	LoadedSize          int64         `json:"loaded_size,omitempty"`
	TorrentSize         int64         `json:"torrent_size,omitempty"`
	PreloadedBytes      int64         `json:"preloaded_bytes,omitempty"`
	PreloadSize         int64         `json:"preload_size,omitempty"`
	DownloadSpeed       float64       `json:"download_speed,omitempty"`
	UploadSpeed         float64       `json:"upload_speed,omitempty"`
	TotalPeers          int           `json:"total_peers,omitempty"`
	PendingPeers        int           `json:"pending_peers,omitempty"`
	ActivePeers         int           `json:"active_peers,omitempty"`
	ConnectedSeeders    int           `json:"connected_seeders,omitempty"`
	HalfOpenPeers       int           `json:"half_open_peers,omitempty"`
	BytesWritten        int64         `json:"bytes_written,omitempty"`
	BytesWrittenData    int64         `json:"bytes_written_data,omitempty"`
	BytesRead           int64         `json:"bytes_read,omitempty"`
	BytesReadData       int64         `json:"bytes_read_data,omitempty"`
	BytesReadUsefulData int64         `json:"bytes_read_useful_data,omitempty"`
	ChunksWritten       int64         `json:"chunks_written,omitempty"`
	ChunksRead          int64         `json:"chunks_read,omitempty"`
	ChunksReadUseful    int64         `json:"chunks_read_useful,omitempty"`
	ChunksReadWasted    int64         `json:"chunks_read_wasted,omitempty"`
	PiecesDirtiedGood   int64         `json:"pieces_dirtied_good,omitempty"`
	PiecesDirtiedBad    int64         `json:"pieces_dirtied_bad,omitempty"`
	DurationSeconds     float64       `json:"duration_seconds,omitempty"`
	BitRate             string        `json:"bit_rate,omitempty"`
	FileStats           []TorrentFile `json:"file_stats,omitempty"`
}

// UnmarshalJSON keeps malformed file-list errors attributable to the API field
// while retaining a fully typed torrent response for callers.
func (torrent *Torrent) UnmarshalJSON(data []byte) error {
	type torrentWire Torrent

	var response struct {
		torrentWire
		FileStats json.RawMessage `json:"file_stats"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return err
	}

	decoded := Torrent(response.torrentWire)
	if len(response.FileStats) > 0 && string(response.FileStats) != "null" {
		if err := json.Unmarshal(response.FileStats, &decoded.FileStats); err != nil {
			return fmt.Errorf("parse torrent file list: %w", err)
		}
	}

	*torrent = decoded

	return nil
}

// TorrentFile identifies one streamable file in a torrent.
type TorrentFile struct {
	ID     int    `json:"id,omitempty"`
	Path   string `json:"path,omitempty"`
	Length int64  `json:"length,omitempty"`
}

// AddTorrentRequest contains metadata accepted by the add action.
type AddTorrentRequest struct {
	Link     string
	Title    string
	Poster   string
	Category string
	Data     string
	Save     bool
}

// UploadTorrentRequest contains one local torrent file and its metadata fields.
type UploadTorrentRequest struct {
	FilePath string
	Title    string
	Poster   string
	Category string
	Data     string
	Save     bool
}

// Settings is a consumer-owned dynamic settings document. Values returns a copy
// so command formatting cannot mutate the decoded response retained by a caller.
type Settings struct {
	values map[string]any
}

// SettingsPatch is a validated partial settings update prepared by the CLI.
type SettingsPatch map[string]any

func (settings *Settings) UnmarshalJSON(data []byte) error {
	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}

	settings.values = values

	return nil
}

func (settings *Settings) MarshalJSON() ([]byte, error) {
	if settings == nil {
		return []byte("null"), nil
	}

	return json.Marshal(settings.values)
}

// Values returns an independent copy of the decoded settings document.
func (settings *Settings) Values() map[string]any {
	if settings == nil {
		return nil
	}

	values := make(map[string]any, len(settings.values))
	for key, value := range settings.values {
		values[key] = cloneJSONValue(value)
	}

	return values
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, nested := range typed {
			cloned[key] = cloneJSONValue(nested)
		}

		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for index, nested := range typed {
			cloned[index] = cloneJSONValue(nested)
		}

		return cloned
	default:
		return typed
	}
}

// Users maps usernames to their server-reported creation timestamps.
type Users map[string]string

// ShutdownTokenStatus reports whether the server has a configured token.
type ShutdownTokenStatus struct {
	Configured bool `json:"configured"`
}

// GeneratedShutdownToken contains a newly generated token returned exactly once.
type GeneratedShutdownToken struct {
	Status string `json:"status"`
	Token  string `json:"token"` // #nosec G117 -- the API intentionally returns a newly generated token once.
}

// ShutdownRequest selects local or public shutdown authorization.
type ShutdownRequest struct {
	Mode   string
	Reason string
	Token  string // #nosec G117 -- caller-provided secret sent only in an authorization header.
}
