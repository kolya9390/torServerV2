package cliapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"server/internal/apiclient"
)

func cmdSettingsDef(cli settingsAPI, opts globalOptions) error {
	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	if err := cli.ResetSettings(ctx); err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		map[string]any{"action": "settings_reset"},
		"OK: settings reset to defaults",
	)
}

func readSettingsPayload(fileSystem FileSystem, jsonRaw, filePath string) (map[string]any, error) {
	if strings.TrimSpace(jsonRaw) == "" && strings.TrimSpace(filePath) == "" {
		return nil, errors.New("settings set requires --json or --file")
	}

	var data []byte

	var err error

	switch {
	case strings.TrimSpace(jsonRaw) != "":
		data = []byte(jsonRaw)
	default:
		data, err = fileSystem.ReadFile(filePath)

		if err != nil {
			return nil, fmt.Errorf("read settings file: %w", err)
		}
	}

	var out map[string]any

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	if err := decoder.Decode(&out); err != nil {
		return nil, fmt.Errorf("parse settings json: %w", err)
	}

	if err := ensureJSONDocumentEnd(decoder); err != nil {
		return nil, err
	}

	return normalizeSettingsPatch(out)
}

func ensureJSONDocumentEnd(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return fmt.Errorf("parse settings json: %w", err)
		}

		return errors.New("parse settings json: multiple JSON values")
	}

	return nil
}

func cmdSettingsGet(cli settingsAPI, opts globalOptions, key string) error {
	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	settings, err := cli.GetSettings(ctx)
	if err != nil {
		return err
	}

	out := settings.Values()

	if key != "" {
		// Get single key
		field := findFieldByKey(key)

		if field == nil {
			// Try direct lookup
			value, ok := out[key]

			if !ok {
				return fmt.Errorf("setting %q not found", key)
			}

			if opts.Output == outputJSON {
				return writeJSONSuccess(opts.stdoutWriter(), map[string]any{"key": key, "value": value})
			}

			_, err := fmt.Fprintf(opts.stdoutWriter(), "%s = %v\n", key, value)

			return err
		}

		value, ok := out[field.Key]

		if !ok {
			return fmt.Errorf("setting %q not found", field.Key)
		}

		if opts.Output == outputJSON {
			return writeJSONSuccess(opts.stdoutWriter(), map[string]any{"key": field.Key, "value": value})
		}

		_, err := fmt.Fprintf(
			opts.stdoutWriter(),
			"%s = %s (%s, %s)\n",
			field.Key,
			formatSettingValue(field, value),
			field.typeLabel(),
			field.accessLabel(),
		)

		return err
	}

	// Print all settings
	if opts.Output == "json" {
		return writeJSONSuccess(opts.stdoutWriter(), out)
	}

	return printSettingsTable(opts.stdoutWriter(), out)
}

func cmdSettingsSetKeyValue(cli settingsAPI, opts globalOptions, key, value string) error {
	field := findFieldByKey(key)
	if field == nil {
		return fmt.Errorf("unknown setting %q. Run '%s settings get' to see available settings", key, opts.programName())
	}

	if field.ReadOnly {
		return readOnlySettingError(*field)
	}

	parsed, err := parseSettingValue(*field, value)

	if err != nil {
		return fmt.Errorf("invalid value for %s: %w", field.Key, err)
	}

	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	patch := apiclient.SettingsPatch{field.Key: parsed}
	if err := cli.SetSettings(ctx, patch); err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		map[string]any{"action": "setting_updated", "key": field.Key, "value": parsed},
		fmt.Sprintf("OK: %s = %s", field.Key, formatSettingValue(field, parsed)),
	)
}

func normalizeSettingsPatch(patch map[string]any) (map[string]any, error) {
	if patch == nil {
		return nil, errors.New("settings patch must be a JSON object")
	}

	if len(patch) == 0 {
		return nil, errors.New("settings patch must contain at least one field")
	}

	normalized := make(map[string]any, len(patch))

	for key, value := range patch {
		field := findFieldByKey(key)
		if field == nil {
			return nil, fmt.Errorf("unknown setting %q", key)
		}

		if field.ReadOnly {
			return nil, readOnlySettingError(*field)
		}

		typed, err := normalizeJSONSettingValue(*field, value)
		if err != nil {
			return nil, fmt.Errorf("invalid value for %s: %w", field.Key, err)
		}

		normalized[field.Key] = typed
	}

	return normalized, nil
}

func normalizeJSONSettingValue(field settingsField, value any) (any, error) {
	var normalized any

	switch field.Kind {
	case settingBool:
		parsed, ok := value.(bool)
		if !ok {
			return nil, errorsExpected("JSON boolean")
		}

		normalized = parsed
	case settingInt, settingDurationSeconds, settingDurationMinutes, settingDurationMillis:
		parsed, ok := integerSettingValue(value)
		if !ok || int64(int(parsed)) != parsed {
			return nil, errorsExpected("JSON integer")
		}

		normalized = int(parsed)
	case settingInt64, settingBytes:
		parsed, ok := integerSettingValue(value)
		if !ok {
			return nil, errorsExpected("JSON integer")
		}

		normalized = parsed
	case settingString:
		parsed, ok := value.(string)
		if !ok {
			return nil, errorsExpected("JSON string")
		}

		normalized = parsed
	case settingStringList, settingObject, settingObjectList:
		if err := validateStructuredSetting(field.Kind, value); err != nil {
			return nil, err
		}

		normalized = value
	default:
		return nil, fmt.Errorf("unsupported setting type %q", field.Kind)
	}

	if err := field.validate(normalized); err != nil {
		return nil, err
	}

	return normalized, nil
}

func readOnlySettingError(field settingsField) error {
	return fmt.Errorf("setting %s is read-only at runtime; %s", field.Key, field.Guidance)
}
