# Research: System-proxy awareness for alks-go

Phase 0 of the plan. Each entry: decision, rationale, alternatives. Evidence was gathered on 2026-11-06 (proof-of-concept programs on macOS and Windows, Docker Desktop on macOS, package metadata from `proxy.golang.org`, and source reads). The proof-of-concept results are recorded in the problem spec's investigation folder.

## R-1. JavaScript engine for PAC evaluation (macOS only)

**Decision**: `github.com/robertkrimen/otto` **v0.5.1** (tagged, 2024-11-05, `go 1.22`), behind an internal package `internal/pac` that also provides the standard PAC helper functions.

**Rationale**:
- PAC files are ES3/ES5 scripts. otto implements ES5 in pure Go, which is enough.
- Constitution II (no cgo) and VI (few, pure-Go dependencies): otto is pure Go and has a short dependency list (`golang.org/x/text`, `gopkg.in/sourcemap.v1`).
- It keeps the module's Go floor at **1.22**, which matches the other known consumer (`actions-mapper`, `go 1.22`). The provider is on 1.25.
- It has proper tagged releases, unlike goja.
- Only macOS needs it: Windows evaluates the PAC natively in WinHTTP (R-3).

**Alternatives considered**:
- **goja**, current head (`dop251/goja`, 2026-11-05): its `go.mod` requires **Go 1.26**, newer than the provider's 1.25. Rejected.
- **goja at `447a94be`** (2026-10-10, the last revision on `go 1.25.0`): workable, and faster than otto. But it forces every consumer onto Go 1.25, pins an untagged revision that will soon be stale, and pulls more dependencies (regexp2, sourcemap, semver, go-yaml, pprof). Kept as the fallback if otto proves too slow.
- **`darren/gpac`**: used in the macOS proof of concept and correct, but last published 2021 and pins a 2021 goja. *Corrected during implementation (2026-11-06):* gpac's JavaScript helper library is extracted from Mozilla's `ProxyAutoConfig.cpp` (MPL 2.0), not gpac's own MIT code, so it was **not** ported. The PAC helper functions in `internal/pac/helpers.go` are original Go implementations of the Netscape PAC functions.
- **`saucelabs/forwarder/pac`**: maintained and goja-based, but pulls in the whole forwarder module (57 requirements). Rejected under Constitution VI.
- **`mattn/go-ieproxy` on macOS**: its macOS code requires cgo, and with `CGO_ENABLED=0` it silently returns an empty configuration. Rejected (Constitution II).

**Risk**: otto is slower than goja. The proof of concept measured 27–41 ms per evaluation with goja for a 55 KB Zscaler PAC; otto may be several times slower. Mitigated by per-host caching (R-5). Measure in the integration test, and switch to the goja fallback only if a single evaluation exceeds the time limit in R-6.

## R-2. macOS: reading the system proxy

**Decision**: run `/usr/sbin/scutil --proxy` and parse its dictionary output. Read the PAC settings (`ProxyAutoConfigEnable`, `ProxyAutoConfigURLString`), the static HTTPS proxy (`HTTPSEnable`, `HTTPSProxy`, `HTTPSPort`) and its exceptions (`ExceptionsList`, `ExcludeSimpleHostnames`). Keep the parser in an untagged file so it is unit-tested on every platform against fixture text. Only the command call is darwin-only.

**Rationale**: pure Go, no cgo, available on every macOS. The proof of concept read this machine's Zscaler PAC (`http://127.0.0.1:9000/...`) correctly.

**Alternatives**: calling CFNetwork through `ebitengine/purego` (no cgo, but low-level, adds a dependency, harder to test); cgo (Constitution II).

**PAC fetch**: an HTTP GET of the PAC URL with **no proxy** and a short timeout (R-6). Zscaler serves it from loopback.

## R-3. Windows: reading the system proxy

**Decision**: `github.com/mattn/go-ieproxy` v0.0.12. Call `GetConf()`, then `conf.Automatic.FindProxyForURL(url)` for the PAC or automatic-detection result, then `conf.Static` for the static proxy and bypass list.

**Rationale**:
- Pure Go on Windows: syscalls to `winhttp.dll` and the registry, no cgo. Built with cgo off and verified on two affected Windows laptops on 2026-11-06, where it found the Zscaler PAC, chose `PROXY 127.0.0.1:9000` for ALKS and reached ALKS.
- Its `findProxyForURL` already keeps only the text before the first `;`, which satisfies first-entry-only (FR-003).
- An empty result means `DIRECT`.

**Note**: do **not** use `ieproxy.GetProxyFunc()`. Its own precedence (environment first, then PAC, then static) is close to ours, but it would hide the decision source needed for FR-019 and could not apply `NO_PROXY` to an OS-discovered proxy (FR-004). We call the lower-level functions and apply our own selection.

**Dependency floor**: go-ieproxy needs `golang.org/x/net` ≥ v0.25.0 and `golang.org/x/sys` ≥ v0.20.0. The provider already requires newer versions, so its minimum-version selection is unaffected.

## R-4. Linux, FreeBSD and other platforms

**Decision**: an OS source that reports "nothing configured". The selection then reduces to environment variables, else direct, which is today's behaviour (FR-009, problem spec R13). No GNOME, KDE or `/etc` reading.

## R-5. Caching

**Decision**:
- Read the OS configuration **once per client**, at construction.
- Cache the PAC script per client, and cache the PAC **decision per host** (scheme plus host plus port) for the client's lifetime.
- The cache is safe for concurrent use.
- No re-reading during a client's life. Terraform runs are short, and a new client sees new settings.

**Rationale**: SC-007; PAC evaluation costs tens of milliseconds, while an ALKS run makes many requests to one host.

## R-6. Time limits and failure handling

**Decision**:
- PAC fetch: 5 s timeout.
- PAC evaluation: 2 s budget per host, enforced with otto's interrupt channel.
- `scutil`: 5 s.
- Any failure in these steps is a **discovery failure**: fall back to the environment, then direct, and log at debug level (FR-006).
- A proxy that *was* chosen but refuses connections is **not** a discovery failure. The request fails, and the error names the proxy (FR-003).
- A PAC answer that is malformed, empty or of an unsupported type (`SOCKS`, `HTTPS`) is treated as `DIRECT`, with a debug line.

## R-7. Applying NO_PROXY to OS-discovered proxies

**Decision**: use `golang.org/x/net/http/httpproxy`. Build a `httpproxy.Config{HTTPSProxy: <chosen>, NoProxy: <NO_PROXY from the environment>}` and call its `ProxyFunc()`, so Go's own matching rules apply, including the `localhost` and loopback exemption (FR-004).

**Rationale**: these are exactly the rules users already get from `HTTPS_PROXY`, with no separate implementation. `httpproxy` is in `golang.org/x/net`, which is already a transitive requirement through go-ieproxy.

**Note**: this reads `NO_PROXY` through Go's standard mechanism, which Constitution V explicitly allows. The library defines no environment variables of its own.

## R-8. Options API shape

**Decision**: functional options, `type ClientOption func(*clientConfig)`, as trailing variadic parameters on `NewClient`, `NewSTSClient` and `NewBearerTokenClient`. Options are applied **before** `NewSTSClient` makes its constructor request (FR-014).

**Rationale**: adding a final variadic parameter is source-compatible for every existing call (FR-012, Constitution I). Functional options are the idiomatic Go pattern for this.

**Compatibility caveat**: code that stores a constructor in a variable of the old function type, for example `var f func(string, string, string, string) (*alks.Client, error) = alks.NewSTSClient`, would no longer compile. A code search of the known consumers (the provider, `actions-mapper`) is done in the tasks before release. Judged acceptable, and documented in the changelog.

## R-9. Logging

**Decision**:
- Remove the `httputil.DumpRequest(req, true)` block in `NewRequest`. Replace it with one debug line: method, URL path and, after the response, status. Never the headers or body (FR-018).
- Add a `redactURL` helper that strips userinfo from proxy URLs.
- The proxy decision is logged once per client, after the first decision (FR-019).
- Keep using the standard `log` package, which the provider already captures under `TF_LOG`.

## R-10. The HTTP transport

**Decision**: keep `go-cleanhttp` (an existing dependency). Build `cleanhttp.DefaultTransport()` and set its `Proxy` to our selection function. If a caller supplies `WithHTTPClient`, use it unchanged and skip the proxy machinery (FR-015).

## R-11. Release workflow

**Decision**: add `.github/workflows/release-auto.yml` and `.github/workflows/check-for-release-label.yml`, modelled on the provider's. The release job:
1. reads the merged PR's label;
2. bumps the semver from the latest `v*` tag;
3. creates an annotated tag;
4. pushes it to GHE **and** to github.com.

Also create the four `release/*` labels and add `CHANGELOG.md`.

**Why push to github.com directly**: `alks-go` has only one secret, `GH_COM_TOKEN`, and no PAT for GHE. A tag pushed by the workflow's built-in `GITHUB_TOKEN` does not start other workflows, so `mirror.yml` would not run for it. The release job therefore pushes the tag to github.com itself with `GH_COM_TOKEN`, as `mirror.yml` does. It also runs the same `HEAD`-already-tagged guard as the provider, and checks out the PR's merge commit.

**Branch protection**: `master` requires one review and the `build` check. Adding the label check as a required check is a repository setting, left to Dave.

**`v1.0.0`**: tag the current `master` (`a8e45b1`) by hand, before the proxy PR merges. Pushing the tag to GHE runs `mirror.yml`, because `mirror.yml` triggers on tags.

## R-12. Module Go version

**Decision**: raise the `go` directive from `1.16` to **`1.22`**, required by otto. Keep vendoring.

**Rationale**: consumers on Go 1.21 or older would have to upgrade to use v1.1.0. The known consumers are on 1.22 (`actions-mapper`) and 1.25 (the provider). Call it out in the changelog.
