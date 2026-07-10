package api

import (
	"encoding/json"
	"reflect"
	"strings"

	"server/internal/app/contracts"
)

func mergeSettingsPatch(
	current *contracts.Settings,
	patch *contracts.Settings,
	rawPatch json.RawMessage,
) (*contracts.Settings, error) {
	if patch == nil {
		return nil, newValidationError("sets", "is required for action=set")
	}

	if !hasRawJSONValue(rawPatch) {
		return patch, nil
	}

	presentFields, err := settingsPatchFields(rawPatch)
	if err != nil {
		return nil, err
	}

	if current == nil {
		current = &contracts.Settings{}
	}

	merged := *current
	applyPresentSettingsFields(&merged, patch, presentFields)

	return &merged, nil
}

func settingsPatchFields(rawPatch json.RawMessage) (map[string]struct{}, error) {
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(rawPatch, &rawFields); err != nil {
		return nil, newValidationError("sets", "must be a json object")
	}

	if rawFields == nil {
		return nil, newValidationError("sets", "must be a json object")
	}

	fields := make(map[string]struct{}, len(rawFields))
	for name := range rawFields {
		fields[name] = struct{}{}
	}

	return fields, nil
}

func applyPresentSettingsFields(
	dst *contracts.Settings,
	src *contracts.Settings,
	presentFields map[string]struct{},
) {
	dstValue := reflect.ValueOf(dst).Elem()
	srcValue := reflect.ValueOf(src).Elem()
	dstType := dstValue.Type()

	for i := range dstValue.NumField() {
		field := dstType.Field(i)
		if field.PkgPath != "" || !settingsFieldPresent(field.Name, presentFields) {
			continue
		}

		dstValue.Field(i).Set(srcValue.Field(i))
	}
}

func settingsFieldPresent(name string, presentFields map[string]struct{}) bool {
	if _, ok := presentFields[name]; ok {
		return true
	}

	for present := range presentFields {
		if strings.EqualFold(present, name) {
			return true
		}
	}

	return false
}
