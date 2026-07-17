package apiclient

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

const (
	versionPath                  = "/api/v1/version"
	expectedProduct              = "torrserver"
	supportedAPIVersion          = "v1"
	requiredManagementCapability = "management-api-v1"
	legacyRootIdentifier         = "legacy-root"
	unknownApplicationVersion    = "unknown"
)

// CompatibilityErrorKind is a stable machine-readable compatibility failure category.
type CompatibilityErrorKind string

const (
	CompatibilityEndpointMissing CompatibilityErrorKind = "compatibility_endpoint_missing"
	CompatibilityAuthentication  CompatibilityErrorKind = "compatibility_authentication_failed"
	CompatibilityMalformed       CompatibilityErrorKind = "compatibility_document_malformed"
	CompatibilityWrongProduct    CompatibilityErrorKind = "not_torrserver"
	CompatibilityUnsupportedAPI  CompatibilityErrorKind = "unsupported_api_version"
	CompatibilityTransport       CompatibilityErrorKind = "compatibility_transport_error"
	CompatibilityServer          CompatibilityErrorKind = "compatibility_server_error"
)

// CompatibilityError explains why a reachable endpoint cannot be used as a compatible TorrServer.
type CompatibilityError struct {
	Kind       CompatibilityErrorKind
	Product    string
	APIVersion string
	Cause      error
}

func (err *CompatibilityError) Error() string {
	switch err.Kind {
	case CompatibilityEndpointMissing:
		return "TorrServer compatibility endpoint /api/v1/version is unavailable; " +
			"the server may be outdated or --server may point to a different service"
	case CompatibilityAuthentication:
		return "TorrServer compatibility check was rejected; verify --user and TS_PASSWORD"
	case CompatibilityMalformed:
		return "TorrServer compatibility response is malformed: " + compatibilityCause(err.Cause)
	case CompatibilityWrongProduct:
		if strings.TrimSpace(err.Product) == "" {
			return "configured endpoint is not TorrServer: product marker is missing"
		}

		return fmt.Sprintf("configured endpoint is not TorrServer: product is %q", err.Product)
	case CompatibilityUnsupportedAPI:
		return fmt.Sprintf(
			"unsupported TorrServer API version %q; this torrctl supports %q",
			err.APIVersion,
			supportedAPIVersion,
		)
	case CompatibilityTransport:
		return "cannot reach TorrServer compatibility endpoint: " + compatibilityCause(err.Cause)
	default:
		return "TorrServer compatibility check failed: " + compatibilityCause(err.Cause)
	}
}

func (err *CompatibilityError) Unwrap() error {
	return err.Cause
}

func normalizeAndValidateVersion(version *Version) error {
	if version == nil {
		return malformedCompatibilityError("version document is missing")
	}

	if isRecognizedLegacyVersion(*version) {
		version.Product = expectedProduct
		version.ApplicationVersion = unknownApplicationVersion
		version.LegacyContract = true

		return nil
	}

	product := strings.TrimSpace(version.Product)
	if product != expectedProduct {
		return &CompatibilityError{Kind: CompatibilityWrongProduct, Product: product}
	}

	apiVersion := strings.TrimSpace(version.Current)
	if apiVersion == "" {
		return malformedCompatibilityError("current API version is missing")
	}

	if apiVersion != supportedAPIVersion {
		return &CompatibilityError{Kind: CompatibilityUnsupportedAPI, APIVersion: apiVersion}
	}

	if strings.TrimSpace(version.ApplicationVersion) == "" {
		return malformedCompatibilityError("application version is missing")
	}

	if !slices.Contains(version.Capabilities, requiredManagementCapability) {
		return malformedCompatibilityError(
			fmt.Sprintf("required capability %q is missing", requiredManagementCapability),
		)
	}

	return nil
}

func isRecognizedLegacyVersion(version Version) bool {
	return strings.TrimSpace(version.Product) == "" &&
		strings.TrimSpace(version.ApplicationVersion) == "" &&
		len(version.Capabilities) == 0 &&
		strings.TrimSpace(version.Current) == supportedAPIVersion &&
		slices.Contains(version.Deprecated, legacyRootIdentifier) &&
		strings.TrimSpace(version.Deprecation) != "" &&
		strings.TrimSpace(version.Sunset) != ""
}

func classifyCompatibilityError(err error) error {
	var responseErr *ResponseError
	if errors.As(err, &responseErr) {
		switch responseErr.StatusCode {
		case http.StatusNotFound, http.StatusMethodNotAllowed:
			return &CompatibilityError{Kind: CompatibilityEndpointMissing, Cause: err}
		case http.StatusUnauthorized, http.StatusForbidden:
			return &CompatibilityError{Kind: CompatibilityAuthentication, Cause: err}
		default:
			return &CompatibilityError{Kind: CompatibilityServer, Cause: err}
		}
	}

	var decodeErr *ResponseDecodeError
	if errors.As(err, &decodeErr) {
		return &CompatibilityError{Kind: CompatibilityMalformed, Cause: err}
	}

	var limitErr *ResponseLimitError
	if errors.As(err, &limitErr) {
		return &CompatibilityError{Kind: CompatibilityMalformed, Cause: err}
	}

	return &CompatibilityError{Kind: CompatibilityTransport, Cause: err}
}

func malformedCompatibilityError(message string) error {
	return &CompatibilityError{
		Kind:  CompatibilityMalformed,
		Cause: errors.New(message),
	}
}

func compatibilityCause(err error) string {
	if err == nil {
		return "unknown error"
	}

	return sanitizeErrorText(err.Error())
}
