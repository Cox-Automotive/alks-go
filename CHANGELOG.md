# Changelog

Releases are tags on `ghe.coxautoinc.com/ETS-CloudAutomation/alks-go`, mirrored to `github.com/Cox-Automotive/alks-go` (the module path). Label a pull request `release/patch`, `release/minor`, `release/major` or `release/skip`; merging it tags the next version. Add an entry here in the same pull request.

## 1.1.0

System-proxy awareness (Rally F325998).

FEATURES:

* **Proxy auto-discovery, on by default (`ProxyAuto`).** For each ALKS request the client uses:
  1. `HTTPS_PROXY`, filtered by `NO_PROXY`, if set (unchanged from before);
  2. otherwise the operating system's proxy configuration. On **macOS** that is the `scutil --proxy` settings, with the PAC file evaluated in pure Go. On **Windows** it is WinHTTP / Internet Options, including PAC and automatic detection;
  3. otherwise a direct connection.

  On Linux, FreeBSD and other platforms there is no OS step, so behaviour is unchanged.
* **PAC and `NO_PROXY` rules.** Only the first entry of a PAC answer is used. If that proxy is unreachable the request fails with an error naming it, and does not fall back to `DIRECT`. `NO_PROXY` also exempts hosts from an OS-discovered proxy. `localhost` and loopback addresses are never proxied.
* **Windows automatic detection.** When only "Automatically detect settings" is on and detection finds nothing (for example off the corporate network), a manual proxy is used if one is configured, as Windows does. An explicit PAC that answers `DIRECT` is honoured.
* **Discovery failures are not fatal, and not permanent.** If the OS settings or the PAC cannot be read or evaluated, the client falls back to the environment, then direct, logs a `[WARN]` line, and tries again after 30 seconds. Successful decisions, the OS settings and the PAC script are re-checked every 5 minutes, so a long-running process picks up network changes.
* **PAC files are treated as untrusted.** Every evaluation is bounded: a 2-second budget that also covers the script's top level, a JavaScript stack-depth limit (runaway recursion is an error, not a crash), and abandoning a built-in that ignores the budget. The PAC file is limited to 2 MiB. `weekdayRange` and `timeRange` are implemented; `dateRange` always returns true.
* **OS exception lists** accept IP addresses and CIDR ranges, including the macOS short form `169.254/16`.
* **New constructor options** (trailing, variadic; existing calls compile unchanged):
  * `WithProxyMode(ProxyAuto | ProxyEnvironment | ProxyNone)`
  * `WithProxyURL(url)`: an explicit proxy for every request; `NO_PROXY` does not apply
  * `WithUserAgent(ua)`
  * `WithHTTPClient(*http.Client)`: used as is; combining it with `WithProxyMode` or `WithProxyURL` is an error
* **Options apply to the constructor's own request.** `NewSTSClient` makes one request (`GET /loginRoles/id/me`), and options now apply to it. Before, that request always went out with User-Agent `alks-go`.

SECURITY:

* Requests and responses are no longer logged in full. Previously every request, including authentication headers, and every response body, which can include session credentials, was written to the log. Now one redacted line is logged per request (method and path) and per response (status), plus one line per client recording the proxy decision. Credentials in proxy URLs are removed from logs and errors, including the error for a proxy URL that cannot be parsed or has no `scheme://` (for example `user:pass@host:port`). User information in PAC answers is removed from logs.

BREAKING / NOTES:

* **Minimum Go version is now 1.22** (was 1.16), required by the PAC engine (`github.com/robertkrimen/otto`).
* **The default proxy behaviour changes on macOS and Windows** when no `HTTPS_PROXY` applies. To get exactly the old behaviour, pass `WithProxyMode(ProxyEnvironment)`.
* The constructors gained a trailing variadic parameter. Direct calls are unaffected. Code that stores a constructor in a variable of the old function type would no longer compile. The known consumers (`ETS-CloudAutomation/terraform-provider-alks`, `MAN-VehicleInformationRTC/actions-mapper`) only call the constructors directly (checked 2026-11-06).
* New dependencies, vendored: `github.com/robertkrimen/otto` v0.5.1, `github.com/mattn/go-ieproxy` v0.0.12, `golang.org/x/net` (for `http/httpproxy`).

## 1.0.0

Baseline: the first tagged release, equal to `master` before 1.1.0. Before this, alks-go had no tags and consumers pinned commits (pseudo-versions).
