package reporting

import (
	"errors"
	"net/url"
)

// DefaultEndpoint is frostyard's problem intake endpoint
// (frostyard/problem-intake docs/specs/intake-api.md). A test never sends to
// it: every test uses an httptest server.
const DefaultEndpoint = "https://report.frostyard.org/v1/report"

// PrivacyURL is the page that explains what a report holds.
const PrivacyURL = "https://frostyard.org/privacy/problem-reports/"

// ParseEndpoint checks a configured endpoint and returns it normalized. An
// empty value is DefaultEndpoint. An endpoint must be an absolute http or
// https URL with a host, no user information and no fragment.
func ParseEndpoint(s string) (string, error) {
	if s == "" {
		return DefaultEndpoint, nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", errors.New("reporting: the report endpoint is not a URL: " + err.Error())
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("reporting: the report endpoint must use https or http")
	}
	if u.Host == "" {
		return "", errors.New("reporting: the report endpoint has no host")
	}
	if u.User != nil {
		return "", errors.New("reporting: the report endpoint must not carry user information")
	}
	if u.Fragment != "" || u.RawFragment != "" {
		return "", errors.New("reporting: the report endpoint must not have a fragment")
	}
	return u.String(), nil
}
