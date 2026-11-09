package alks

import (
	"bufio"
	"bytes"
	"strings"
)

// parseScutilProxy parses the output of macOS `scutil --proxy` into an
// osProxyConfig. It lives in an untagged file so it is tested on every platform.
func parseScutilProxy(out []byte) osProxyConfig {
	var cfg osProxyConfig
	values := map[string]string{}
	var inArray string

	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line == "<dictionary> {" {
			continue
		}
		if line == "}" {
			inArray = ""
			continue
		}
		key, value, found := strings.Cut(line, " : ")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if inArray != "" {
			if inArray == "ExceptionsList" {
				cfg.exceptions = append(cfg.exceptions, value)
			}
			continue
		}
		if strings.HasPrefix(value, "<array>") {
			inArray = key
			continue
		}
		values[key] = value
	}

	if values["ProxyAutoConfigEnable"] == "1" {
		cfg.pacURL = values["ProxyAutoConfigURLString"]
	}
	if values["HTTPSEnable"] == "1" && values["HTTPSProxy"] != "" {
		port := values["HTTPSPort"]
		if port == "" {
			port = "443"
		}
		cfg.staticHTTPS = values["HTTPSProxy"] + ":" + port
	}
	cfg.excludeSimpleHostnames = values["ExcludeSimpleHostnames"] == "1"
	return cfg
}
