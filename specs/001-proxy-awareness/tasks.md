---
description: "Task list for system-proxy awareness in alks-go"
---

# Tasks: System-proxy awareness for alks-go

**Input**: Design documents from `specs/001-proxy-awareness/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/public-api.md, quickstart.md

**Tests**: Required. Constitution Principle III requires tests for every behaviour change, and the spec's success criteria are verified by them. Write each story's tests first and see them fail.

**Organization**: grouped by user story (US1–US5 from spec.md). Paths are repository-relative (flat `alks` package at the root, plus `internal/pac/`).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel (different files, no dependency on an unfinished task)
- **[Story]**: US1–US5

---

## Phase 1: Setup

**Purpose**: versioning baseline, dependencies, labels

- [ ] T001 **(manual, Dave)** Tag the current `master` (`a8e45b1`) as annotated `v1.0.0` on GHE and push the tag; confirm `mirror.yml` copies it to github.com and that `https://proxy.golang.org/github.com/!cox-!automotive/alks-go/@v/v1.0.0.info` resolves (FR-020)
- [x] T002 Create the labels `release/patch`, `release/minor`, `release/major`, `release/skip` on `ETS-CloudAutomation/alks-go` (FR-021, FR-022)
- [x] T003 Raise the `go` directive in `go.mod` from `1.16` to `1.22` (research R-12)
- [x] T004 Add `github.com/robertkrimen/otto v0.5.1`, `github.com/mattn/go-ieproxy v0.0.12` and `golang.org/x/net` (for `http/httpproxy`) to `go.mod`; run `go mod tidy` and `go mod vendor`; commit `vendor/` (research R-1, R-3, R-7; Constitution VI)
- [x] T005 [P] Create `testdata/` with fixtures: `scutil-pac.txt` (PAC enabled, loopback URL), `scutil-static.txt` (static HTTPS proxy with `ExceptionsList`), `scutil-none.txt`, `pac-zscaler-like.js` (ALKS host to `PROXY`, `login.microsoftonline.com` to `DIRECT`, a `PROXY a; DIRECT` list, a `dnsDomainIs` and an `isInNet` rule), `pac-broken.js`, `pac-slow.js` (a busy loop)

---

## Phase 2: Foundational (blocks every story)

**Purpose**: the options type, the OS-source seam, and the PAC evaluator everything else uses

- [x] T006 Create `options.go`:
  - `ProxyMode` (`ProxyAuto`, `ProxyEnvironment`, `ProxyNone`) with `String()`;
  - `ClientOption` and the unexported `clientConfig`;
  - `WithProxyMode`, `WithProxyURL`, `WithUserAgent`, `WithHTTPClient`;
  - validation: an unknown mode, a malformed or non-http(s) proxy URL, and `WithProxyURL` with `ProxyNone` are all errors (contracts/public-api.md, data-model.md; FR-010, FR-011, FR-012)
- [x] T007 [P] Create `osproxy.go`: the `osProxySource` interface (`config()`, `pacProxyFor(*url.URL)`), the `OSProxyConfig` type, and a package-level factory that tests can replace (data-model.md)
- [x] T008 [P] Create `osproxy_other.go` (`//go:build !darwin && !windows`): a source that reports nothing configured (FR-009, research R-4)
- [x] T009 [P] Create `internal/pac/helpers.go`: the standard PAC functions (`isPlainHostName`, `dnsDomainIs`, `localHostOrDomainIs`, `isResolvable`, `isInNet`, `dnsResolve`, `myIpAddress`, `dnsDomainLevels`, `shExpMatch`, `weekdayRange`, `dateRange`, `timeRange`), written as original Go implementations (gpac's helpers turned out to be Mozilla MPL-2.0 code, so they were not ported; research R-1)
- [x] T010 Create `internal/pac/pac.go`, which depends on T009:
  - `Evaluate(script, url string, budget time.Duration) (string, error)` on otto, enforcing the budget through otto's interrupt channel;
  - `FirstEntry(answer string)`, which parses `PROXY h:p`, `DIRECT`, and lists separated by `;`, and treats empty, malformed, `SOCKS` and `HTTPS` answers as `DIRECT` (FR-003, research R-6)
- [x] T011 [P] Create `logging.go`: `redactURL` (strips userinfo) and a `debugf` helper on the standard `log` package (FR-018, FR-019, research R-9)
- [x] T012 [P] Tests `internal/pac/pac_test.go`: each helper; every `testdata/pac-*.js` fixture; first-entry parsing including `PROXY a:9000; DIRECT`; the budget firing on `pac-slow.js`; `pac-broken.js` returning an error
- [x] T013 [P] Tests `options_test.go`: defaults (`ProxyAuto`, User-Agent `alks-go`), each option, every validation error (FR-010 to FR-013)

**Checkpoint**: options, OS seam and PAC evaluator exist and are tested; nothing is wired into the client yet.

---

## Phase 3: User Story 1 — ALKS calls follow the system proxy (P1) 🎯 MVP

**Goal**: a default client sends ALKS requests through the proxy the OS (PAC or static) names, with nothing configured.

**Independent Test**: with a fake OS source pointing at a loopback PAC that proxies the ALKS host to a loopback recording proxy, a default client's request reaches the recording proxy; a `DIRECT` host does not.

### Tests (write first, see them fail)

- [x] T014 [P] [US1] `scutil_parse_test.go`: parse each `testdata/scutil-*.txt` into the expected `OSProxyConfig` (runs on all platforms)
- [x] T015 [P] [US1] `proxy_test.go`, auto-mode OS cases:
  - a PAC `PROXY` answer is used;
  - a PAC `DIRECT` answer goes direct;
  - per-host PAC rules are honoured;
  - a static HTTPS proxy is used when there is no PAC;
  - a static-proxy exception goes direct;
  - `excludeSimpleHostnames` is honoured;
  - the decision source is recorded as `pac` or `os-static`.

  Covers FR-001, FR-002, FR-009.
- [x] T016 [P] [US1] `proxy_integration_test.go`: an `httptest` PAC server plus a recording `CONNECT` proxy on `127.0.0.1` and a fake OS source. Covers quickstart table rows 1, 3 and 7: proxied; an unreachable first entry fails with an error naming the proxy and no direct attempt (FR-003); 1,000 requests to one host evaluate the PAC once (SC-007)

### Implementation

- [x] T017 [US1] Create `scutil_parse.go` (untagged): parse `scutil --proxy` dictionary output into `OSProxyConfig` (research R-2)
- [x] T018 [US1] Create `osproxy_darwin.go`:
  - run `/usr/sbin/scutil --proxy` with a 5 s timeout and parse it with T017;
  - fetch the PAC URL with **no proxy** and a 5 s timeout, and cache the script;
  - `pacProxyFor` evaluates with `internal/pac` under a 2 s budget and returns the first entry.

  Research R-2, R-6.
- [x] T019 [P] [US1] Create `osproxy_windows.go`: `ieproxy.GetConf()` mapped to `OSProxyConfig` (PAC URL, auto-detect, static `https` proxy, bypass list); `pacProxyFor` uses `conf.Automatic.FindProxyForURL`, where empty means `DIRECT` (research R-3)
- [x] T020 [US1] Create `proxy.go`, which depends on T006, T007, T010 and T011:
  - the selection function implementing data-model.md "Selection rules";
  - the per-client decision cache, keyed by scheme+host+port and mutex-protected;
  - the transport is `cleanhttp.DefaultTransport()` with `Proxy` set to the selection function.

  FR-001 to FR-003, research R-5, R-10.
- [x] T021 [US1] Wire the transport into the client in `api.go`: replace `cleanhttp.DefaultClient()` in all three constructors with a client built from `clientConfig` (depends on T020)

**Checkpoint**: US1 tests pass; a default client follows the OS PAC or static proxy.

---

## Phase 4: User Story 2 — existing behaviour and explicit settings still win (P1)

**Goal**: `HTTPS_PROXY`, `NO_PROXY`, Linux, containers and CI behave exactly as before; the modes and explicit URL work; discovery failures never fail a request.

**Independent Test**: the full selection matrix (mode × environment × OS settings × discovery failure) matches expected decisions.

### Tests (write first)

- [x] T022 [P] [US2] `proxy_test.go`, matrix cases:
  - `HTTPS_PROXY` set beats the OS proxy (`source=env`);
  - lowercase `https_proxy` is honoured;
  - `NO_PROXY` matching the ALKS host skips the OS PAC;
  - `NO_PROXY` rules: `*`, a leading dot, CIDR, a port;
  - localhost and loopback are never proxied;
  - `ProxyEnvironment` ignores the OS;
  - `ProxyNone` ignores `HTTPS_PROXY`;
  - an explicit URL is used and `NO_PROXY` is not applied.

  Covers FR-004, FR-005, FR-010, FR-011.
- [x] T023 [P] [US2] `proxy_test.go`, failure cases:
  - an OS config error falls back to the environment, then direct, without an error;
  - a PAC fetch failure falls back;
  - a PAC evaluation timeout or error falls back;
  - each fallback is recorded in the decision.

  Covers FR-006.
- [x] T024 [P] [US2] `proxy_integration_test.go`: quickstart rows 2, 4 and 5 (`NO_PROXY` exemption, PAC server down falls back to direct, `ProxyEnvironment` ignores the PAC)
- [x] T025 [P] [US2] A test, run on every platform with the real (non-fake) source for that OS where the platform is linux or freebsd, confirming no proxy is reported and the behaviour equals `http.ProxyFromEnvironment` (FR-009, problem spec R13)

### Implementation

- [x] T026 [US2] In `proxy.go`:
  - environment step through `httpproxy.FromEnvironment().ProxyFunc()`;
  - a `NO_PROXY` check against OS-discovered proxies through an `httpproxy.Config` built from the chosen proxy plus `NO_PROXY`;
  - the `ProxyEnvironment` (`http.ProxyFromEnvironment`) and `ProxyNone` (nil proxy) paths;
  - an explicit URL bypasses `NO_PROXY`.

  Research R-7.
- [x] T027 [US2] In `proxy.go`: discovery-failure handling. On any OS or PAC error, skip the OS steps, decide direct, and record `fallback` (FR-006, research R-6)

**Checkpoint**: US1 and US2 pass together; existing behaviour is preserved.

---

## Phase 5: User Story 3 — options, compatibility, first request (P2)

**Goal**: existing calls compile; options apply from the first request, including the one inside `NewSTSClient`; a custom HTTP client is used as is.

**Independent Test**: the constructor's own request carries the configured User-Agent, against a local test server.

### Tests (write first)

- [x] T028 [P] [US3] `api_test.go`: the existing test calls compile unchanged against the new signatures (SC-002). Add an `httptest` server that records the User-Agent of `GET /rest/loginRoles/id/me` made by `NewSTSClient(..., WithUserAgent("x"))`; expect `x`. Without the option, expect `alks-go` (FR-014)
- [x] T029 [P] [US3] `api_test.go`: `WithHTTPClient` is used as is, with its transport's `Proxy` untouched; `SetUserAgent` still affects later requests (FR-015)
- [x] T030 [P] [US3] `api_test.go`: constructor validation errors surface from each constructor (FR-011)

### Implementation

- [x] T031 [US3] `api.go`:
  - add `opts ...ClientOption` to `NewClient`, `NewSTSClient` and `NewBearerTokenClient`;
  - build `clientConfig` and apply the User-Agent and transport **before** `NewSTSClient` calls `GetMyLoginRole`;
  - return validation errors;
  - keep ignoring a failed login-role lookup, as today.

  FR-012, FR-014, FR-017.
- [x] T032 [US3] Search the known consumers (`ETS-CloudAutomation/terraform-provider-alks`, `MAN-VehicleInformationRTC/actions-mapper`) for constructors held in function-typed variables, which the variadic change would break (research R-8); record the result in `CHANGELOG.md`

**Checkpoint**: US1–US3 pass; the API is backward compatible.

---

## Phase 6: User Story 4 — no credentials in logs (P2)

**Goal**: logs help troubleshooting and never carry credentials.

**Independent Test**: log capture across basic, STS and bearer requests contains zero credential values, plus exactly one proxy-decision line.

### Tests (write first)

- [x] T033 [P] [US4] `logging_test.go`: capture `log` output while making requests with basic, STS and bearer credentials against `httptest`, with a proxy URL containing userinfo; assert that no password, key, token or `Authorization` value appears (SC-004)
- [x] T034 [P] [US4] `logging_test.go`: one proxy-decision line per client, with mode, source and redacted host; a fallback line on discovery failure (FR-019)

### Implementation

- [x] T035 [US4] `api.go`: remove the `httputil.DumpRequest(req, true)` block in `NewRequest`; log one redacted debug line (method and path) instead (FR-018, research R-9)
- [x] T036 [US4] `proxy.go`: log the decision once per client through `debugf`, with `redactURL` (FR-019)

**Checkpoint**: US1–US4 pass.

---

## Phase 7: User Story 5 — tagged releases (P3)

**Goal**: label-driven semver releases from GHE, mirrored to github.com, with a changelog. This work releases as `v1.1.0`.

**Independent Test**: a PR without a release label fails the check; merging with `release/minor` produces `v1.1.0` on GHE and github.com.

- [x] T037 [P] [US5] Create `.github/workflows/check-for-release-label.yml`, modelled on the provider's: it fails unless the PR has `release/patch`, `release/minor`, `release/major` or `release/skip` (FR-022)
- [x] T038 [P] [US5] Create `.github/workflows/release-auto.yml`, modelled on the provider's `release-auto.yml`. On a merged PR to `master` labelled `release/patch`, `release/minor` or `release/major`, it:
  - checks out the PR's **merge commit**;
  - refuses to run if HEAD already carries a `v*` tag;
  - bumps from the latest `v*` tag;
  - creates an annotated tag;
  - pushes it to GHE;
  - **also pushes it to github.com with `GH_COM_TOKEN`**, because a tag pushed with the built-in token does not start `mirror.yml`.

  Pin actions to SHAs, as the repository already does. Covers FR-021 and research R-11.
- [x] T039 [P] [US5] Create `CHANGELOG.md` with entries for `1.0.0` (baseline, first tag) and `1.1.0` (this work: the new default, the three modes, the options, the first-request User-Agent fix, the logging fix, the `go 1.22` minimum, how to restore old behaviour with `WithProxyMode(ProxyEnvironment)`, the result of T032) (FR-023)
- [ ] T040 [US5] **(manual, Dave)** Optionally make `check_for_release_label` a required status check on `master` (repository setting)

**Checkpoint**: all stories complete.

---

## Phase 8: Polish & cross-cutting

- [x] T041 [P] `README.md`: a proxy section (default, modes, explicit URL, `NO_PROXY`, opting out, the container note `http://host.docker.internal:9000`); a section on the options; correct the outdated `NewSTSClient` example (5 arguments shown, 4 real); remove the dead Travis badge (FR-024)
- [ ] T042 [P] Cloud Platform Docs (`ETS-CloudAutomation/Cloud-Platform-Docs`, `app/docs/01_tools_by_cloud_provider/01_aws/06_security_services/02_alks/04_alks_clients/06_alks_go.md`): document the default, the modes and the options. Separate PR in that repository, opened alongside the release (FR-024)
- [x] T043 Run the no-cgo cross-build matrix from quickstart.md §2; every target builds (SC-006)
- [x] T044 Run `gofmt -l .`, `go vet ./...` and `go test ./...` (with `GOTOOLCHAIN=go1.25.0` on recent macOS); all clean
- [x] T045 Check the PAC timing with otto against `testdata/pac-zscaler-like.js` in the integration test; if a single evaluation exceeds the 2 s budget, switch to the goja fallback in research R-1 and re-run T043
- [ ] T046 Tell the `actions-mapper` owners about the new default, the opt-out, and the `go 1.22` minimum before `v1.1.0` is tagged (problem spec open item 4)
- [ ] T047 Open the PR with the `release/minor` label; on merge confirm `v1.1.0` on GHE, github.com and `proxy.golang.org` (quickstart §5, SC-008); report the tag to the provider work

---

## Dependencies & Execution Order

### Phase dependencies

- **Setup (Phase 1):** T001 must happen before the PR merges, not before coding starts. T003 and T004 come before any code that imports the new dependencies.
- **Foundational (Phase 2):** depends on T003 and T004, and blocks all stories.
- **US1 (Phase 3):** depends on Phase 2.
- **US2 (Phase 4):** depends on US1's `proxy.go` (T020), since it extends the same selection function.
- **US3 (Phase 5):** depends on T021, the transport wiring.
- **US4 (Phase 6):** T035 is independent of the proxy work and can be done any time after Phase 2. T036 depends on T020.
- **US5 (Phase 7):** independent of the code. It can run in parallel with Phases 3–6.
- **Polish (Phase 8):** after the stories. T047 is last.

### Within each story

Tests first, and confirm they fail. Then parsers and sources, then the selection, then the wiring.

## Parallel examples

```text
Phase 2:  T007, T008, T009, T011 together; then T010; T012 and T013 together
US1:      T014, T015, T016 together; T017 then T018; T019 in parallel with T017 and T018
US5:      T037, T038, T039 together, any time
```

## Implementation strategy

1. **MVP = Phases 1–4.** US1 and US2 are both P1: the fix plus no regressions. Stop and validate with the integration test.
2. **Then US3.** The provider needs `WithUserAgent`, `WithProxyMode` and `WithProxyURL`.
3. **Then US4.** The security fix ships in the same release.
4. **US5 any time,** but before the PR merges.
5. **Release:** T046, then T047, as `v1.1.0`. The provider SDD starts from that tag.

## Traceability

| Spec | Tasks |
|---|---|
| FR-001, FR-002 | T015, T016, T018, T019, T020 |
| FR-003 | T010, T012, T016, T020 |
| FR-004, FR-005 | T022, T026 |
| FR-006 | T023, T024, T027 |
| FR-007 | T020 (cleanhttp transport, default TLS) |
| FR-008 | T021 |
| FR-009 | T008, T017, T018, T019, T025 |
| FR-010 to FR-013 | T006, T013, T022, T030, T031 |
| FR-014, FR-015 | T028, T029, T031 |
| FR-016 | T026 (only standard variables, through `httpproxy`) |
| FR-017 | T028, T031, T032 |
| FR-018, FR-019 | T011, T033 to T036 |
| FR-020 to FR-023 | T001, T002, T037 to T040, T047 |
| FR-024 | T041, T042 |
| SC-006 | T043 |
| SC-007 | T016 (1,000-request case), T020 cache |
