# Implementation Plan: System-proxy awareness for alks-go

**Branch**: `feat/proxy-awareness` | **Date**: 2026-11-06 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/001-proxy-awareness/spec.md`

## Summary

alks-go gains a proxy-selection layer for its ALKS HTTP client:

- **Selection order** in the default `auto` mode: the standard `HTTPS_PROXY`/`NO_PROXY`; then the OS proxy configuration (the PAC answer for the request URL, then the OS static proxy); then direct.
- **OS discovery:**
  - macOS: parse `scutil --proxy` and evaluate the PAC in pure Go (otto plus ported PAC helpers);
  - Windows: WinHTTP through `mattn/go-ieproxy`;
  - all other platforms: report nothing, which is today's behaviour.
- **Configuration:** three modes (`auto`, `environment`, `none`) and an explicit proxy URL, supplied by consumers through new functional options on the existing constructors. A custom User-Agent and a custom HTTP client are options too.
- **First request:** options apply to the request made inside `NewSTSClient`.
- **Logging:** the full-request dump in `NewRequest` is replaced by a redacted line, and the proxy decision is logged once per client.
- **Releases:** the repository gains a label-driven release workflow, a release-label check and a changelog. `v1.0.0` is tagged on today's master, and this work releases as `v1.1.0`.

## Technical Context

**Language/Version**: Go. The module `go` directive goes from 1.16 to **1.22** (research R-12). CI builds with the version in `go.mod`.

**Primary Dependencies**:
- existing: `github.com/hashicorp/go-cleanhttp` (transport base);
- new:
  - `github.com/robertkrimen/otto` v0.5.1 (PAC evaluation, macOS path);
  - `github.com/mattn/go-ieproxy` v0.0.12 (Windows);
  - `golang.org/x/net/http/httpproxy` (`NO_PROXY` matching).
- All vendored.

**Storage**: N/A.

**Testing**: the standard `testing` package plus `net/http/httptest`: a local PAC server and a local recording proxy (handles `CONNECT`). OS discovery sits behind an interface with a fake for tests. The scutil parser is tested with fixture text on every platform. Run locally with `GOTOOLCHAIN=go1.25.0 go test ./...` on recent macOS (see `CLAUDE.md`).

**Target Platform**: library, built by consumers with `CGO_ENABLED=0` for windows, linux, darwin and freebsd on amd64, 386, arm and arm64.

**Project Type**: Go library (single module, flat package `alks`, plus `internal/`).

**Performance Goals**: no measurable cost per request after the first request to a host (cached decision). The first PAC evaluation per host stays within its time budget (R-6).

**Constraints**:
- no cgo;
- no environment variables of our own;
- no credentials in logs;
- backward-compatible API;
- discovery failures never fail a request;
- time limits: PAC fetch 5 s, evaluation 2 s, `scutil` 5 s.

**Scale/Scope**: about 8 new source files, about 6 new test files, 2 workflows, a changelog and README changes. Three existing constructor signatures gain variadic options.

## Constitution Check

*GATE: checked before Phase 0 and again after Phase 1 design.*

| Principle | Gate | Result |
|---|---|---|
| I. Backward-compatible API | Existing constructor calls compile unchanged; no exported identifier removed or changed | **Pass.** Trailing variadic options only (R-8). The one caveat is function-typed variables holding a constructor; it is searched for in known consumers and noted in the changelog. The default proxy behaviour changes by design (it is the feature) and is documented, with `ProxyEnvironment` as the way back. |
| II. Pure Go, no cgo | Every target builds with `CGO_ENABLED=0` | **Pass.** otto, go-ieproxy (Windows syscalls) and `scutil` through `os/exec` are all cgo-free. A cross-build matrix task proves it. |
| III. Tested, no live services | No real ALKS, AWS or network beyond loopback; no dependence on the real OS proxy | **Pass.** A fake OS source in tests; PAC server and recording proxy on loopback; the scutil parser tested from fixtures. |
| IV. No secrets in logs | No credentials in log or error output | **Pass.** The full-request dump is removed; proxy URLs are redacted; a capture test asserts zero credentials (SC-004). |
| V. Configuration through code | The library reads no variables of its own | **Pass.** Only Go's standard `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY`, through `httpproxy`, which the constitution allows. |
| VI. Few, vendored, pure-Go dependencies | Each new dependency justified | **Pass with justification.** otto: PAC on macOS; heavier options rejected (R-1). go-ieproxy: Windows proxy, proven on affected laptops (R-3). x/net: already transitive, and gives Go's own `NO_PROXY` rules (R-7). All vendored. |
| VII. Semantic versioning with tags | Release as a tag, label-driven, with a changelog | **Pass.** `v1.0.0` on today's master, then `v1.1.0` from this PR with `release/minor` (R-11). |

Re-check after design: unchanged. No violations, so the Complexity Tracking section is empty.

## Project Structure

### Documentation (this feature)

```text
specs/001-proxy-awareness/
├── spec.md
├── plan.md              # this file
├── research.md          # Phase 0
├── data-model.md        # Phase 1
├── quickstart.md        # Phase 1
├── contracts/
│   └── public-api.md    # Phase 1: exported API added by this feature
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
api.go                      # constructors take ...ClientOption; options applied before the constructor request; DumpRequest removed
options.go                  # NEW ClientOption, WithProxyMode, WithProxyURL, WithUserAgent, WithHTTPClient, ProxyMode constants, validation
proxy.go                    # NEW transport construction; selection (env → OS PAC → OS static → direct); NO_PROXY via httpproxy; per-host cache; decision log
osproxy.go                  # NEW osProxySource interface + OSProxyConfig type; shared helpers
osproxy_darwin.go           # NEW scutil call + PAC fetch + internal/pac evaluation
osproxy_windows.go          # NEW go-ieproxy GetConf / FindProxyForURL mapping
osproxy_other.go            # NEW //go:build !darwin && !windows — reports nothing
scutil_parse.go             # NEW untagged parser for `scutil --proxy` output
logging.go                  # NEW redactURL, debug helpers
internal/pac/pac.go         # NEW otto-based evaluator with time budget; first-entry parsing
internal/pac/helpers.go     # NEW PAC helper functions ported from darren/gpac (MIT, attributed)
internal/pac/pac_test.go    # NEW
options_test.go             # NEW
proxy_test.go               # NEW selection matrix, NO_PROXY, fallback, cache
scutil_parse_test.go        # NEW fixture-based
proxy_integration_test.go   # NEW httptest PAC server + recording CONNECT proxy; first-request User-Agent
logging_test.go             # NEW credential capture test
testdata/                   # NEW scutil fixtures, PAC fixtures
.github/workflows/release-auto.yml            # NEW
.github/workflows/check-for-release-label.yml # NEW
CHANGELOG.md                # NEW
README.md                   # proxy section, options, opt-out; fix outdated NewSTSClient example
go.mod / go.sum / vendor/   # go 1.22; new dependencies vendored
```

**Structure Decision**: keep the flat `alks` package (the repository's existing layout) and add one internal package for PAC evaluation, so it can be tested in isolation without growing the public API. Platform code uses file suffixes (`_darwin`, `_windows`) and a build tag for the rest (Constitution II).

## Dependencies on the provider (not delivered here)

- The provider pins `github.com/Cox-Automotive/alks-go v1.1.0` and re-vendors.
- It maps `proxy_mode`/`proxy_url` and its environment variables to `WithProxyMode`/`WithProxyURL`.
- It passes its User-Agent string through `WithUserAgent`.
- Covered by the separate terraform-provider-alks SDD (problem spec, section "SDD scope for terraform-provider-alks").

## Complexity Tracking

None. No constitution violations.
