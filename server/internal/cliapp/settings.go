package cliapp

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

type settingValueKind string

const (
	settingBool             settingValueKind = "bool"
	settingInt              settingValueKind = "int"
	settingInt64            settingValueKind = "int64"
	settingString           settingValueKind = "string"
	settingBytes            settingValueKind = "bytes"
	settingDurationSeconds  settingValueKind = "duration-seconds"
	settingDurationMinutes  settingValueKind = "duration-minutes"
	settingDurationMillis   settingValueKind = "duration-milliseconds"
	settingStringList       settingValueKind = "string-list"
	settingObject           settingValueKind = "object"
	settingObjectList       settingValueKind = "object-list"
	settingsConfigGuidance                   = "change it in config.yml and restart the server"
	settingsStorageGuidance                  = "use the storage settings API instead"
)

// settingsField is the CLI contract for one field exposed by /api/v1/settings.
type settingsField struct {
	Key         string
	Kind        settingValueKind
	Unit        string
	Description string
	ReadOnly    bool
	Guidance    string
	Min         int64
	Max         int64
	Allowed     []string
}

// knownSettingsFields intentionally mirrors contracts.Settings. Keep the
// reflection-based coverage test green when the server contract changes.
var knownSettingsFields = []settingsField{
	{Key: "CacheSize", Kind: settingBytes, Unit: "bytes", Description: "memory cache size", Min: 1},
	{Key: "ReaderReadAHead", Kind: settingInt, Unit: "%", Description: "reader read-ahead", Min: 5, Max: 100},
	{Key: "PreloadCache", Kind: settingInt, Unit: "%", Description: "preload target", Max: 100},
	{Key: "UseDisk", Kind: settingBool, Description: "use disk cache"},
	{Key: "TorrentsSavePath", Kind: settingString, Description: "disk cache path"},
	{Key: "RemoveCacheOnDrop", Kind: settingBool, Description: "remove cache when torrent is dropped"},
	{Key: "ForceEncrypt", Kind: settingBool, Description: "require encrypted peer connections"},
	{Key: "RetrackersMode", Kind: settingInt, Description: "retracker policy: 0=off, 1=add, 2=remove, 3=replace", Max: 3},
	{Key: "TorrentDisconnectTimeout", Kind: settingDurationSeconds, Unit: "seconds", Description: "idle torrent disconnect timeout", Min: 1},
	{Key: "EnableDebug", Kind: settingBool, Description: "debug endpoints and logging", ReadOnly: true, Guidance: settingsConfigGuidance},
	{Key: "ServiceOnlyDebug", Kind: settingBool, Description: "service-only debug logging", ReadOnly: true, Guidance: settingsConfigGuidance},
	{Key: "DebugEstablishedConnsOverride", Kind: settingInt, Description: "debug established-peer override", ReadOnly: true, Guidance: settingsConfigGuidance},
	{Key: "DebugTotalHalfOpenConnsOverride", Kind: settingInt, Description: "debug half-open-peer override", ReadOnly: true, Guidance: settingsConfigGuidance},
	{Key: "DebugTrackerBudgetOverride", Kind: settingInt, Description: "debug tracker budget override", ReadOnly: true, Guidance: settingsConfigGuidance},
	{Key: "DebugStablePeerCap", Kind: settingInt, Description: "debug stable-peer cap", ReadOnly: true, Guidance: settingsConfigGuidance},
	{Key: "EnableDLNA", Kind: settingBool, Description: "enable DLNA server"},
	{Key: "FriendlyName", Kind: settingString, Description: "DLNA friendly name"},
	{Key: "EnableRutorSearch", Kind: settingBool, Description: "enable Rutor search"},
	{Key: "EnableTorznabSearch", Kind: settingBool, Description: "enable Torznab search"},
	{Key: "TorznabUrls", Kind: settingObjectList, Description: "Torznab endpoints as a JSON array"},
	{Key: "TMDBSettings", Kind: settingObject, Description: "TMDB settings as a JSON object"},
	{Key: "EnableIPv6", Kind: settingBool, Description: "enable IPv6"},
	{Key: "DisableTCP", Kind: settingBool, Description: "disable TCP transport"},
	{Key: "DisableUTP", Kind: settingBool, Description: "disable uTP transport"},
	{Key: "DisableUPNP", Kind: settingBool, Description: "disable UPnP port mapping"},
	{Key: "DisableDHT", Kind: settingBool, Description: "disable DHT"},
	{Key: "DisablePEX", Kind: settingBool, Description: "disable peer exchange"},
	{Key: "DisableUpload", Kind: settingBool, Description: "disable torrent upload"},
	{Key: "DownloadRateLimit", Kind: settingInt, Unit: "KB/s", Description: "download limit; 0=unlimited"},
	{Key: "UploadRateLimit", Kind: settingInt, Unit: "KB/s", Description: "upload limit; 0=unlimited"},
	{Key: "ConnectionsLimit", Kind: settingInt, Description: "maximum connections per torrent", Min: 1},
	{Key: "PeersListenPort", Kind: settingInt, Description: "peer listen port; 0=automatic", Max: 65535},
	{Key: "EnableLPD", Kind: settingBool, Description: "enable local peer discovery"},
	{Key: "LPDIPv6", Kind: settingBool, Description: "enable IPv6 local peer discovery"},
	{Key: "SslPort", Kind: settingInt, Description: "HTTPS listen port; 0=disabled", Max: 65535},
	{Key: "SslCert", Kind: settingString, Description: "TLS certificate path"},
	{Key: "SslKey", Kind: settingString, Description: "TLS private key path"},
	{Key: "ResponsiveMode", Kind: settingBool, Description: "enable responsive reader mode"},
	{
		Key:         "CoreProfile",
		Kind:        settingString,
		Description: "torrent engine tuning profile",
		Allowed:     []string{"custom", "low-end", "low-cpu", "balanced", "tcp-only-balanced", "high-throughput", "nas"},
	},
	{Key: "MaxConcurrentStreams", Kind: settingInt, Description: "stream limit; 0=automatic"},
	{Key: "StreamQueueSize", Kind: settingInt, Description: "stream queue size; 0=automatic"},
	{Key: "StreamQueueWaitSec", Kind: settingDurationSeconds, Unit: "seconds", Description: "maximum stream queue wait", Min: 1},
	{Key: "MaxUniquePlaybackTorrents", Kind: settingInt, Description: "unique playback torrent limit; 0=unlimited"},
	{Key: "StartupPreloadPolicy", Kind: settingString, Description: "startup preload policy", Allowed: []string{"skip-active", "legacy"}},
	{Key: "AdaptiveRAMinMB", Kind: settingInt, Unit: "MB", Description: "minimum adaptive read-ahead; 0=default"},
	{Key: "AdaptiveRAMaxMB", Kind: settingInt, Unit: "MB", Description: "maximum adaptive read-ahead; 0=default"},
	{Key: "WarmDiskCacheSizeMB", Kind: settingInt64, Unit: "MB", Description: "warm disk cache size; 0=disabled"},
	{Key: "WarmDiskCacheTTLMin", Kind: settingDurationMinutes, Unit: "minutes", Description: "warm disk cache retention; 0=default"},
	{Key: "DiskSyncPolicy", Kind: settingString, Description: "disk sync policy", Allowed: []string{"none", "periodic", "always"}},
	{Key: "DiskSyncIntervalMS", Kind: settingDurationMillis, Unit: "milliseconds", Description: "periodic disk sync interval", Min: 1},
	{Key: "DiskWriteBatchSize", Kind: settingInt, Description: "sequential disk write batch size", Min: 1},
	{Key: "MetadataWorkers", Kind: settingInt, Description: "metadata workers; 0=automatic"},
	{Key: "MetadataQueueSize", Kind: settingInt, Description: "metadata queue size; 0=automatic"},
	{Key: "PreloadWorkers", Kind: settingInt, Description: "preload workers; 0=automatic"},
	{Key: "PreloadQueueSize", Kind: settingInt, Description: "preload queue size; 0=automatic"},
	{Key: "ShowFSActiveTorr", Kind: settingBool, Description: "debug filesystem activity", ReadOnly: true, Guidance: settingsConfigGuidance},
	{Key: "StoreSettingsInJSON", Kind: settingBool, Description: "settings storage backend", ReadOnly: true, Guidance: settingsStorageGuidance},
	{Key: "StoreViewedInJSON", Kind: settingBool, Description: "view history storage backend", ReadOnly: true, Guidance: settingsStorageGuidance},
	{Key: "EnableProxy", Kind: settingBool, Description: "enable P2P proxy"},
	{Key: "ProxyHosts", Kind: settingStringList, Description: "proxy host patterns as a JSON array"},
}

func findFieldByKey(key string) *settingsField {
	for index := range knownSettingsFields {
		if strings.EqualFold(knownSettingsFields[index].Key, key) {
			return &knownSettingsFields[index]
		}
	}

	return nil
}

func parseSettingValue(field settingsField, value string) (any, error) {
	value = strings.TrimSpace(value)

	var (
		parsed any
		err    error
	)

	switch field.Kind {
	case settingBool:
		parsed, err = parseBoolSetting(value)
	case settingInt:
		parsed, err = strconv.Atoi(value)
	case settingInt64:
		parsed, err = strconv.ParseInt(value, 10, 64)
	case settingBytes:
		parsed, err = parseByteSize(value)
	case settingDurationSeconds:
		parsed, err = parseDurationUnit(value, time.Second)
	case settingDurationMinutes:
		parsed, err = parseDurationUnit(value, time.Minute)
	case settingDurationMillis:
		parsed, err = parseDurationUnit(value, time.Millisecond)
	case settingString:
		parsed = value
	case settingStringList, settingObject, settingObjectList:
		parsed, err = parseStructuredSetting(field.Kind, value)
	default:
		err = fmt.Errorf("unsupported setting type %q", field.Kind)
	}

	if err != nil {
		return nil, fmt.Errorf("expected %s: %w", field.typeLabel(), err)
	}

	if err := field.validate(parsed); err != nil {
		return nil, err
	}

	return parsed, nil
}

func parseBoolSetting(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q; use true or false", value)
	}
}

func parseByteSize(value string) (int64, error) {
	upper := strings.ToUpper(strings.TrimSpace(value))
	multiplier := float64(1)
	number := upper

	for suffix, factor := range map[string]float64{
		"GB": 1024 * 1024 * 1024,
		"MB": 1024 * 1024,
		"KB": 1024,
	} {
		if strings.HasSuffix(upper, suffix) {
			multiplier = factor
			number = strings.TrimSpace(strings.TrimSuffix(upper, suffix))

			break
		}
	}

	parsed, err := strconv.ParseFloat(number, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 {
		return 0, fmt.Errorf("invalid byte size %q", value)
	}

	bytes := parsed * multiplier
	if bytes > math.MaxInt64 || math.Trunc(bytes) != bytes {
		return 0, fmt.Errorf("byte size %q is out of range or not a whole byte", value)
	}

	return int64(bytes), nil
}

func parseDurationUnit(value string, unit time.Duration) (int, error) {
	if plain, err := strconv.Atoi(value); err == nil {
		return plain, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", value)
	}

	if duration%unit != 0 {
		return 0, fmt.Errorf("duration %q must be a whole number of %s", value, durationUnitName(unit))
	}

	result := duration / unit
	if int64(int(result)) != int64(result) {
		return 0, fmt.Errorf("duration %q is out of range", value)
	}

	return int(result), nil
}

func durationUnitName(unit time.Duration) string {
	switch unit {
	case time.Millisecond:
		return "milliseconds"
	case time.Second:
		return "seconds"
	case time.Minute:
		return "minutes"
	default:
		return unit.String()
	}
}

func parseStructuredSetting(kind settingValueKind, value string) (any, error) {
	var parsed any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if err := validateStructuredSetting(kind, parsed); err != nil {
		return nil, err
	}

	return parsed, nil
}

func validateStructuredSetting(kind settingValueKind, value any) error {
	switch kind {
	case settingStringList:
		items, ok := value.([]any)
		if !ok {
			return errorsExpected("JSON array of strings")
		}

		for _, item := range items {
			if _, ok := item.(string); !ok {
				return errorsExpected("JSON array of strings")
			}
		}
	case settingObject:
		if _, ok := value.(map[string]any); !ok {
			return errorsExpected("JSON object")
		}
	case settingObjectList:
		items, ok := value.([]any)
		if !ok {
			return errorsExpected("JSON array of objects")
		}

		for _, item := range items {
			if _, ok := item.(map[string]any); !ok {
				return errorsExpected("JSON array of objects")
			}
		}
	}

	return nil
}

func errorsExpected(expected string) error {
	return fmt.Errorf("expected %s", expected)
}

func (f settingsField) validate(value any) error {
	if len(f.Allowed) > 0 {
		text, ok := value.(string)
		if !ok || !containsFold(f.Allowed, text) {
			return fmt.Errorf("must be one of: %s", strings.Join(f.Allowed, ", "))
		}
	}

	number, ok := integerSettingValue(value)
	if !ok {
		return nil
	}

	if number < f.Min {
		return fmt.Errorf("must be at least %d %s", f.Min, f.Unit)
	}

	if f.Max > 0 && number > f.Max {
		return fmt.Errorf("must be at most %d %s", f.Max, f.Unit)
	}

	return nil
}

func containsFold(values []string, candidate string) bool {
	for _, value := range values {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}

	return false
}

func integerSettingValue(value any) (int64, bool) {
	switch number := value.(type) {
	case int:
		return int64(number), true
	case int64:
		return number, true
	case float64:
		if math.Trunc(number) == number && number >= math.MinInt64 && number <= math.MaxInt64 {
			return int64(number), true
		}
	case json.Number:
		parsed, err := number.Int64()

		return parsed, err == nil
	}

	return 0, false
}

func (f settingsField) typeLabel() string {
	switch f.Kind {
	case settingBytes:
		return "bytes"
	case settingDurationSeconds, settingDurationMinutes, settingDurationMillis:
		return "duration"
	case settingStringList:
		return "string[]"
	case settingObject:
		return "object"
	case settingObjectList:
		return "object[]"
	default:
		return string(f.Kind)
	}
}

func (f settingsField) accessLabel() string {
	if f.ReadOnly {
		return "read-only"
	}

	return "runtime"
}

func formatSettingValue(field *settingsField, value any) string {
	if field != nil {
		if number, ok := integerSettingValue(value); ok {
			if field.Kind == settingBytes {
				return formatByteSize(number)
			}

			if field.Kind == settingInt || field.Kind == settingInt64 || isDurationKind(field.Kind) {
				return strconv.FormatInt(number, 10)
			}
		}
	}

	switch typed := value.(type) {
	case bool:
		return strconv.FormatBool(typed)
	case string:
		if typed == "" {
			return "(empty)"
		}

		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		encoded, err := json.Marshal(value)
		if err == nil {
			return string(encoded)
		}

		return fmt.Sprintf("%v", value)
	}
}

func isDurationKind(kind settingValueKind) bool {
	return kind == settingDurationSeconds || kind == settingDurationMinutes || kind == settingDurationMillis
}

func formatByteSize(value int64) string {
	for _, unit := range []struct {
		name string
		size int64
	}{
		{name: "GB", size: 1024 * 1024 * 1024},
		{name: "MB", size: 1024 * 1024},
		{name: "KB", size: 1024},
	} {
		if value >= unit.size && value%unit.size == 0 {
			return fmt.Sprintf("%d %s", value/unit.size, unit.name)
		}
	}

	return strconv.FormatInt(value, 10)
}

func printSettingsTable(writer io.Writer, settings map[string]any) error {
	table := tabwriter.NewWriter(writer, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "KEY\tVALUE\tTYPE\tUNIT\tACCESS\tDESCRIPTION")

	known := make(map[string]struct{}, len(knownSettingsFields))

	for index := range knownSettingsFields {
		field := &knownSettingsFields[index]
		known[strings.ToLower(field.Key)] = struct{}{}

		value, ok := settings[field.Key]
		if !ok {
			continue
		}

		_, _ = fmt.Fprintf(
			table,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			field.Key,
			formatSettingValue(field, value),
			field.typeLabel(),
			field.Unit,
			field.accessLabel(),
			field.Description,
		)
	}

	unknown := make([]string, 0)

	for key := range settings {
		if _, ok := known[strings.ToLower(key)]; !ok {
			unknown = append(unknown, key)
		}
	}

	sort.Strings(unknown)

	for _, key := range unknown {
		_, _ = fmt.Fprintf(table, "%s\t%s\t?\t\tunknown\t\n", key, formatSettingValue(nil, settings[key]))
	}

	return table.Flush()
}
