package alks

import (
	"log"
	"net/url"
)

// debugf writes a debug line through the standard logger, which consumers such as
// the Terraform provider capture (shown with TF_LOG=DEBUG). Never pass credentials.
func debugf(format string, args ...interface{}) {
	log.Printf("[DEBUG] alks-go: "+format, args...)
}

// warnf writes a warning line through the standard logger. Never pass credentials.
func warnf(format string, args ...interface{}) {
	log.Printf("[WARN] alks-go: "+format, args...)
}

// redactURL returns u as a string with any user information removed, for logging.
func redactURL(u *url.URL) string {
	if u == nil {
		return "direct"
	}
	c := *u
	c.User = nil
	return c.String()
}

// redactRawURL is redactURL for an unparsed string. If the string cannot be
// parsed, it is not echoed at all, because it may contain credentials.
func redactRawURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(unparseable URL)"
	}
	// A value without "scheme://" (for example "user:pass@host:port") parses as an
	// opaque URL whose credentials are not in u.User, so it cannot be redacted
	// safely: never echo it.
	if u.Opaque != "" || u.Host == "" {
		return "(invalid URL)"
	}
	return redactURL(u)
}
