package cliapp

import "strings"

const defaultProgramName = "torrserver"

func normalizeProgramName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}

	for _, symbol := range name {
		if symbol >= 'a' && symbol <= 'z' || symbol >= '0' && symbol <= '9' || symbol == '-' {
			continue
		}

		return ""
	}

	return name
}

func (opts globalOptions) programName() string {
	if opts.runtime == nil || opts.runtime.programName == "" {
		return defaultProgramName
	}

	return opts.runtime.programName
}

func commandText(programName, value string) string {
	return strings.ReplaceAll(value, defaultProgramName, programName)
}
