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
	return redactURL(u)
}
