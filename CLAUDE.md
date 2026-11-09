# CLAUDE.md

Guidance for Claude Code when working in this repository.

## Constitution

The repository's governing principles live in the spec-kit constitution and are imported here:

@.specify/memory/constitution.md

Edit the constitution through `/speckit-constitution` or a PR to `.specify/memory/constitution.md`, not by copying rules into this file.

## What this is

`alks-go` is the Go client library for the ALKS API (Airlift Key Service). The ALKS Terraform provider (`ETS-CloudAutomation/terraform-provider-alks`) vendors it. At least one other team's tool imports it too, so treat the exported API as public (Constitution I).

- Module path: `github.com/Cox-Automotive/alks-go` (unchanged since the GHE migration).
- Source of truth: `https://ghe.coxautoinc.com/ETS-CloudAutomation/alks-go`. `.github/workflows/mirror.yml` pushes `master` and tags to github.com.
- Versioning: semver tags from `v1.0.0` (Constitution VII). Label every PR with exactly one of `release/patch`, `release/minor` or `release/skip` (`check-for-release-label.yml`). `release/major` is refused: Go cannot resolve a v2+ tag without a `/v2` module path. `release-auto.yml` tags the merge commit with a lightweight tag and pushes it to github.com, then GHE. To recover a cancelled or failed release, run it by hand with the PR number. Add a `CHANGELOG.md` entry in the same PR.
- Service catalog: component `CI0042503` (`.cai/catalog.yaml`).

## Commands

```bash
go test ./...                     # what CI runs (.github/workflows/go.yml)
go test -run TestName -v .        # one test
gofmt -l .                        # must print nothing
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...   # cross-compile check (Constitution II)
go mod vendor                     # after changing dependencies; vendor/ is committed
```

The Go version comes from `go.mod` (`go-version-file` in CI).

On recent macOS, test binaries built by older Go toolchains abort with `dyld: missing LC_UUID load command` (seen with Go 1.22.12 on 2026-11-06). Run the tests with a current toolchain, for example `GOTOOLCHAIN=go1.25.0 go test ./...`. This is a local toolchain issue, not a code failure. CI uses the `go` directive in `go.mod` (1.22), so it compiles at the minimum version and catches standard-library APIs newer than 1.22 that a local 1.25 build would accept. CI also runs `gofmt`, `go vet` and the `CGO_ENABLED=0` cross-build for every provider target.

## Layout

- `api.go`: the `Client` type, the constructors (`NewClient`, `NewSTSClient`, `NewBearerTokenClient`, each taking trailing `...ClientOption`), `NewRequest`, response decoding.
- `options.go`: `ClientOption`, `ProxyMode`, `WithProxyMode`, `WithProxyURL`, `WithUserAgent`, `WithHTTPClient`.
- `proxy.go`: the HTTP client and proxy selection (environment, then OS PAC, then OS static proxy, then direct), cached per host. `osproxy*.go`: the OS source per platform (macOS `scutil`, Windows `go-ieproxy`, others none). `pacfile.go` and `internal/pac/`: PAC fetch and pure-Go evaluation (otto). Tests replace the OS source through `newOSProxySource`.
- `credentials.go`: auth injection (`Basic`, `STS`, `Bearer`).
- One file per API area (`iam_role.go`, `iam_user.go`, `session.go`, `login_role.go`, ...), each with a `_test.go`.
- `testutils/`: a small local HTTP server for tests.

## Things to know

- `NewSTSClient` makes an ALKS request (`GET /rest/loginRoles/id/me`) inside the constructor; options are applied before it. `GetMyLoginRole` returns a `*AlksError`: never assign it to a variable of type `error` (a nil pointer becomes a non-nil interface).
- Logging: one redacted line per request and response, and one proxy-decision line per client. Never log headers, bodies or credentials (Constitution IV).
- The library reads no environment variables itself (Constitution V); only Go's standard `HTTPS_PROXY`/`NO_PROXY` handling applies.
- Tests must not resolve real hostnames: PAC fixtures guard `isInNet`/`dnsResolve` behind literal-IP checks, and integration tests use the reserved `.test` domain.
- A change only reaches Terraform users after the provider bumps its pin, re-vendors and releases.

## Spec-kit

Spec-kit (v1.3.2) is installed: skills under `.claude/skills/speckit-*`, templates and scripts under `.specify/`. Specs go in `specs/`. Its plan step may append "Active Technologies" and "Recent Changes" sections to this file; that is expected.
