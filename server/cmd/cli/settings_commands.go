package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

func cmdSettingsDef(cli *apiClient, opts globalOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	payload := map[string]any{"action": "def"}

	if err := cli.doJSON(ctx, "POST", "/api/v1/settings", payload, nil, nil); err != nil {
		return err
	}

	fmt.Println("OK: settings reset to defaults")

	return nil
}

func readSettingsPayload(jsonRaw, filePath string) (map[string]any, error) {
	if strings.TrimSpace(jsonRaw) == "" && strings.TrimSpace(filePath) == "" {
		return nil, errors.New("settings set requires --json or --file")
	}

	var data []byte

	var err error

	switch {
	case strings.TrimSpace(jsonRaw) != "":
		data = []byte(jsonRaw)
	default:
		data, err = os.ReadFile(filePath)

		if err != nil {
			return nil, fmt.Errorf("read settings file: %w", err)
		}
	}

	var out map[string]any

	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse settings json: %w", err)
	}

	return out, nil
}

func cmdSettingsGet(cli *apiClient, opts globalOptions, key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	payload := map[string]any{"action": "get"}

	var out map[string]any

	if err := cli.doJSON(ctx, "POST", "/api/v1/settings", payload, &out, nil); err != nil {
		return err
	}

	if key != "" {
		// Get single key
		field := findFieldByKey(key)

		if field == nil {
			// Try direct lookup
			value, ok := out[key]

			if !ok {
				return fmt.Errorf("setting %q not found", key)
			}

			fmt.Printf("%s = %v\n", key, value)

			return nil
		}

		value, ok := out[field.Key]

		if !ok {
			return fmt.Errorf("setting %q not found", field.Key)
		}

		fmt.Printf("%s = %s (%s)\n", field.Key, formatSettingsValue(value), field.Type)

		return nil
	}

	// Print all settings
	if opts.Output == "json" {
		return printJSON(out)
	}

	return printSettingsTable(out)
}

func cmdSettingsSetKeyValue(cli *apiClient, opts globalOptions, key, value string) error {
	// Find field definition
	field := findFieldByKey(key)

	if field == nil {
		return fmt.Errorf("unknown setting %q. Run 'torrserver settings get' to see available settings", key)
	}

	// Parse value
	parsed, err := parseSettingValue(field.Type, value)

	if err != nil {
		return fmt.Errorf("invalid value for %s: %w", field.Key, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	payload := map[string]any{
		"action": "set",
		"sets":   map[string]any{field.Key: parsed},
	}

	if err := cli.doJSON(ctx, "POST", "/api/v1/settings", payload, nil, nil); err != nil {
		return err
	}

	fmt.Printf("OK: %s = %s\n", field.Key, formatSettingsValue(parsed))

	return nil
}
