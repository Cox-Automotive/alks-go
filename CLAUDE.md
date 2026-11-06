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
- Versioning: none so far (no tags in nine years; consumers pin a pseudo-version). Constitution VII introduces semver tags from `v1.0.0`, label-driven like the provider's releases, with a `CHANGELOG.md`. The release workflow does not exist yet.
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

On recent macOS, test binaries built by older Go toolchains abort with `dyld: missing LC_UUID load command` (seen with Go 1.22.12 on 2026-11-06). Run the tests with a current toolchain, for example `GOTOOLCHAIN=go1.25.0 go test ./...`. This is a local toolchain issue, not a code failure; CI on Linux is unaffected.

## Layout

- `api.go`: the `Client` type, the constructors (`NewClient`, `NewSTSClient`, `NewBearerTokenClient`), `NewRequest`, response decoding. The only HTTP client is created here with `cleanhttp.DefaultClient()`.
- `credentials.go`: auth injection (`Basic`, `STS`, `Bearer`).
- One file per API area (`iam_role.go`, `iam_user.go`, `session.go`, `login_role.go`, ...), each with a `_test.go`.
- `testutils/`: a small local HTTP server for tests.

## Things to know

- `NewSTSClient` makes an ALKS request (`GET /rest/loginRoles/id/me`) inside the constructor, before callers can set a User-Agent or other settings.
- `NewRequest` currently logs the full request, headers and body included, with `httputil.DumpRequest`. That conflicts with Constitution IV: do not extend it, and remove or redact it when touching that code.
- The library reads no environment variables itself (Constitution V); proxy behaviour today is Go's standard `HTTPS_PROXY`/`NO_PROXY` handling through `cleanhttp`.
- A change only reaches Terraform users after the provider bumps its pin, re-vendors and releases.

## Spec-kit

Spec-kit (v1.3.2) is installed: skills under `.claude/skills/speckit-*`, templates and scripts under `.specify/`. Specs go in `specs/`. Its plan step may append "Active Technologies" and "Recent Changes" sections to this file; that is expected.
