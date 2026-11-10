# Changelog

Releases are tags on `ghe.coxautoinc.com/ETS-CloudAutomation/alks-go`, mirrored to `github.com/Cox-Automotive/alks-go` (the module path). Label a pull request `release/patch`, `release/minor`, `release/major` or `release/skip`; merging it tags the next version. Add an entry here in the same pull request.

## 1.1.1

PAC hardening (Rally F325998, from review of the Terraform provider's 2.9.0 pull request).

SECURITY:

* **The PAC script no longer sees the request path or query.** For https requests it is asked about `https://host[:port]/` only, as browsers do. For every scheme, user information (`user:password@`) and the fragment are removed. This applies to the macOS evaluator and the Windows WinHTTP lookup. Decisions were already cached per scheme, host and port, so answers are unchanged.
* **PAC evaluation is checked for memory growth as well as time.**
  * The heap is sampled every 20 ms while a script runs. If it has grown by more than 256 MiB, the script is stopped at its next statement.
  * **Limit:** one call into a native builtin (for example a huge `Array.join`) cannot be interrupted, and can allocate well past the limit before it returns. The evaluation is then abandoned, and the caps below bound how often that can happen, not how many bytes one call allocates.
  * At most four abandoned evaluations (stopped by a limit but still inside a builtin) can be outstanding. Beyond that, new evaluations are refused until they finish. Healthy evaluations running at the same time never count.
* **Backoff after a stall, escalating.**
  * After a timeout, or a stop that left a goroutine behind, the PAC file is not evaluated again for 30 seconds, the same as the existing retry interval.
  * Each consecutive stall doubles that (1, 2, 4 and 8 minutes), up to 15 minutes, so a broken or hostile script cannot leave a new goroutine behind every 30 seconds. A successful evaluation resets it.
  * A memory stop at which the script did stop is not backed off, and neither is a refusal because of other abandoned evaluations; both are retried after the usual 30 seconds. The heap is measured for the whole process, so unrelated allocation must not cause a long backoff.
* **Deeply nested PAC files are rejected before parsing.** otto's parser overflows the Go stack, which kills the process and cannot be recovered, at roughly 225,000 nested `(` or `[`, which fits under the size limit. Nesting deeper than 1,000 levels is now an error. Real PAC files nest a few levels.
* **One PAC evaluation at a time.** The memory check measures the whole process, so a memory-heavy script could stop a healthy evaluation running alongside it. Evaluations take milliseconds, so they now run one after another.
* **PAC fetches no longer block other lookups.**
  * The PAC file is downloaded outside the cache lock, one download per URL at a time; other lookups wait for it, or use the last good script.
  * A failed download or compile is remembered for 30 seconds, so many hosts do not each wait for an unreachable PAC server.
  * If a refresh fails, the last good script stays in use, so requests keep going through the proxy while the PAC server is down.
  * Downloads no longer leave idle keep-alive connections behind.
* **PAC files over 512 KiB are rejected,** not truncated. Previously the limit was 2 MiB and a longer file was cut off.
* **The JavaScript interpreter (otto) is linked only on macOS,** the only platform that evaluates PAC files in Go. Windows, Linux and FreeBSD builds no longer include it.

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

* **STS credentials are no longer forwarded on a redirect to another host.** Go removes `Authorization` on a cross-host redirect but copies custom headers, so a redirect to a different host or port, or from https to http, used to carry the `ALKS-STS-Access-Key`, `ALKS-STS-Secret-Key` and `ALKS-STS-Session-Token` headers to that server. They are now removed, and a warning is logged. Clients built with `WithHTTPClient` keep their own redirect policy.

BUG FIXES:

* A request that fails outright (connection refused, DNS, timeout, unreachable proxy) no longer crashes the program with a nil-pointer panic in `CreateSession`, `CreateIamSession`, `GetLoginRole` or `IsIamEnabled`. It returns an error, and `CreateSession` now reports the actual failure.

BREAKING / NOTES:

* **Minimum Go version is now 1.22** (was 1.16), required by the PAC engine (`github.com/robertkrimen/otto`).
* **The default proxy behaviour changes on macOS and Windows** when no `HTTPS_PROXY` applies. To get exactly the old behaviour, pass `WithProxyMode(ProxyEnvironment)`.
* The constructors gained a trailing variadic parameter. Direct calls are unaffected. Code that stores a constructor in a variable of the old function type would no longer compile. The known consumers (`ETS-CloudAutomation/terraform-provider-alks`, `MAN-VehicleInformationRTC/actions-mapper`) only call the constructors directly (checked 2026-11-06).
* New dependencies, vendored: `github.com/robertkrimen/otto` v0.5.1, `github.com/mattn/go-ieproxy` v0.0.12, `golang.org/x/net` (for `http/httpproxy`).

## 1.0.0

Baseline: the first tagged release, equal to `master` before 1.1.0. Before this, alks-go had no tags and consumers pinned commits (pseudo-versions).
