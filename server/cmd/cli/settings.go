package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// settingsField describes a single BTSets field for CLI display and parsing.
type settingsField struct {
	Key         string
	Type        string // "int", "int64", "bool", "string", "duration"
	Description string
}

// knownSettingsFields lists all known BTSets fields with their types.
var knownSettingsFields = []settingsField{
	// Cache
	{Key: "CacheSize", Type: "int64", Description: "Cache size in bytes"},
	{Key: "ReaderReadAHead", Type: "int", Description: "Read-ahead percentage (5-100)"},
	{Key: "PreloadCache", Type: "int", Description: "Preload cache percentage"},

	// Disk
	{Key: "UseDisk", Type: "bool", Description: "Use disk cache"},
	{Key: "TorrentsSavePath", Type: "string", Description: "Path for disk cache"},
	{Key: "RemoveCacheOnDrop", Type: "bool", Description: "Remove cache on drop"},

	// Torrent
	{Key: "ForceEncrypt", Type: "bool", Description: "Force encryption"},
	{Key: "RetrackersMode", Type: "int", Description: "Retrackers mode (0-3)"},
	{Key: "TorrentDisconnectTimeout", Type: "int", Description: "Disconnect timeout in seconds"},
	{Key: "EnableDebug", Type: "bool", Description: "Enable debug logging"},

	// DLNA
	{Key: "EnableDLNA", Type: "bool", Description: "Enable DLNA server"},
	{Key: "FriendlyName", Type: "string", Description: "DLNA friendly name"},

	// Search
	{Key: "EnableRutorSearch", Type: "bool", Description: "Enable Rutor search"},
	{Key: "EnableTorznabSearch", Type: "bool", Description: "Enable Torznab search"},

	// Network
	{Key: "EnableIPv6", Type: "bool", Description: "Enable IPv6"},
	{Key: "DisableTCP", Type: "bool", Description: "Disable TCP"},
	{Key: "DisableUTP", Type: "bool", Description: "Disable uTP"},
	{Key: "DisableUPNP", Type: "bool", Description: "Disable UPnP"},
	{Key: "DisableDHT", Type: "bool", Description: "Disable DHT"},
	{Key: "DisablePEX", Type: "bool", Description: "Disable PEX"},
	{Key: "DisableUpload", Type: "bool", Description: "Disable upload"},
	{Key: "ConnectionsLimit", Type: "int", Description: "Max connections per torrent"},
	{Key: "DownloadRateLimit", Type: "int", Description: "Download rate limit (KB/s, 0=unlimited)"},
	{Key: "UploadRateLimit", Type: "int", Description: "Upload rate limit (KB/s, 0=unlimited)"},
	{Key: "PeersListenPort", Type: "int", Description: "Peers listen port (0=random)"},

	// Streaming
	{Key: "ResponsiveMode", Type: "bool", Description: "Enable responsive mode"},
	{Key: "MaxConcurrentStreams", Type: "int", Description: "Max concurrent streams (0=unlimited)"},
	{Key: "StreamQueueSize", Type: "int", Description: "Stream queue size"},
	{Key: "StreamQueueWaitSec", Type: "int", Description: "Stream queue wait time (seconds)"},

	// Proxy
	{Key: "EnableProxy", Type: "bool", Description: "Enable proxy"},

	// SSL
	{Key: "SslPort", Type: "int", Description: "SSL port"},
	{Key: "SslCert", Type: "string", Description: "SSL certificate path"},
	{Key: "SslKey", Type: "string", Description: "SSL key path"},
}

// parseSettingValue parses a string value into the appropriate Go type based on the field type.
// It supports human-readable suffixes:
//   - Sizes: KB, MB, GB (e.g., "128MB" → 134217728)
//   - Durations: s, m, h (e.g., "30s" → 30)
func parseSettingValue(fieldType, value string) (any, error) {
	value = strings.TrimSpace(value)

	switch fieldType {
	case "bool":
		switch strings.ToLower(value) {
		case "true", "1", "yes", "on":
			return true, nil
		case "false", "0", "no", "off":
			return false, nil
		default:
			return nil, fmt.Errorf("invalid boolean value %q (use true/false, 1/0, yes/no, on/off)", value)
		}

	case "int":
		// Try duration suffixes first
		if strings.HasSuffix(value, "s") || strings.HasSuffix(value, "m") || strings.HasSuffix(value, "h") {
			d, err := time.ParseDuration(value)
			if err == nil {
				if strings.HasSuffix(value, "s") {
					return int(d.Seconds()), nil
				}

				if strings.HasSuffix(value, "m") {
					return int(d.Minutes()), nil
				}

				if strings.HasSuffix(value, "h") {
					return int(d.Hours()), nil
				}
			}
		}

		// Try plain integer
		n, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("invalid integer value %q: %w", value, err)
		}

		return n, nil

	case "int64":
		// Try size suffixes first (KB, MB, GB)
		upper := strings.ToUpper(value)

		var multiplier int64

		switch {
		case strings.HasSuffix(upper, "GB"):
			multiplier = 1024 * 1024 * 1024
			value = strings.TrimSuffix(upper, "GB")
		case strings.HasSuffix(upper, "MB"):
			multiplier = 1024 * 1024
			value = strings.TrimSuffix(upper, "MB")
		case strings.HasSuffix(upper, "KB"):
			multiplier = 1024
			value = strings.TrimSuffix(upper, "KB")
		default:
			multiplier = 1
		}

		if multiplier > 1 {
			value = strings.TrimSpace(value)
			n, err := strconv.ParseFloat(value, 64)

			if err != nil {
				return nil, fmt.Errorf("invalid size value %q: %w", value, err)
			}

			return int64(n * float64(multiplier)), nil
		}

		// Plain integer
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer value %q: %w", value, err)
		}

		return n, nil

	case "string":
		return value, nil

	default:
		return nil, fmt.Errorf("unknown field type %q", fieldType)
	}
}

// findFieldByKey returns the settings field by key (case-insensitive).
func findFieldByKey(key string) *settingsField {
	for i := range knownSettingsFields {
		if strings.EqualFold(knownSettingsFields[i].Key, key) {
			return &knownSettingsFields[i]
		}
	}

	return nil
}

// formatSettingsValue formats a settings value for display.
func formatSettingsValue(value any) string {
	switch v := value.(type) {
	case bool:
		if v {
			return "true"
		}

		return "false"
	case int64:
		// Try to format as human-readable size
		if v >= 1024*1024*1024 && v%(1024*1024*1024) == 0 {
			return fmt.Sprintf("%d GB", v/(1024*1024*1024))
		}

		if v >= 1024*1024 && v%(1024*1024) == 0 {
			return fmt.Sprintf("%d MB", v/(1024*1024))
		}

		if v >= 1024 && v%1024 == 0 {
			return fmt.Sprintf("%d KB", v/1024)
		}

		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case string:
		if v == "" {
			return "(empty)"
		}

		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

// printSettingsTable prints all settings in a formatted table.
func printSettingsTable(settings map[string]any) error {
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "KEY\tVALUE\tTYPE\tDESCRIPTION")

	for _, field := range knownSettingsFields {
		value, ok := settings[field.Key]
		if !ok {
			continue
		}

		displayValue := formatSettingsValue(value)
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", field.Key, displayValue, field.Type, field.Description)
	}

	// Print any additional fields not in knownSettingsFields
	for key, value := range settings {
		found := false

		for _, field := range knownSettingsFields {
			if strings.EqualFold(field.Key, key) {
				found = true

				break
			}
		}

		if !found {
			_, _ = fmt.Fprintf(w, "%s\t%v\t?\t\n", key, value)
		}
	}

	return w.Flush()
}
