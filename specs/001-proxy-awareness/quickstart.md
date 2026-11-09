# Quickstart: validating system-proxy awareness

Validation scenarios that prove the feature end to end. See [contracts/public-api.md](./contracts/public-api.md) for the API and [data-model.md](./data-model.md) for the selection rules.

## Prerequisites

- Go 1.22 or newer. On recent macOS use `GOTOOLCHAIN=go1.25.0` for tests (see `CLAUDE.md`).
- No network access needed for the automated checks.

## 1. Automated checks (all platforms, CI)

```bash
go test ./...                      # selection matrix, PAC evaluator, scutil parser, logging capture, integration
gofmt -l .                         # prints nothing
go vet ./...
```

Expected: every test passes; `gofmt` prints nothing.

## 2. No-cgo cross-build matrix

```bash
for os in windows linux darwin freebsd; do
  for arch in amd64 386 arm arm64; do
    [ "$os/$arch" = "darwin/386" ] && continue
    CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build ./... || echo "FAIL $os/$arch"
  done
done
```

Expected: no `FAIL` lines. SC-006.

## 3. Integration scenario (automated, loopback only)

`proxy_integration_test.go` starts a PAC server and a recording `CONNECT` proxy on `127.0.0.1`, and injects a fake OS source that points at the PAC. Then:

| Case | Expected |
|---|---|
| Default client, ALKS host proxied by PAC | Request arrives at the recording proxy |
| Same, `NO_PROXY` = ALKS host | Proxy sees nothing |
| PAC `PROXY 127.0.0.1:<closed port>; DIRECT` | Request fails; error names the proxy; nothing goes direct |
| PAC server down | Request goes direct (fallback); debug log records it |
| `WithProxyMode(ProxyEnvironment)` with OS PAC set | PAC ignored |
| `WithUserAgent("x")` on `NewSTSClient` | The constructor's request carries `User-Agent: x` |
| 1,000 requests to one host | PAC evaluated once (SC-007) |

## 4. Optional manual check on a real machine

Not required (problem spec section 6). For a sanity check on an affected laptop, the investigation's `alks-proxy-check` program exercises the same OS discovery. Expected result: direct `403`, system proxy `401` from ALKS.

## 5. Release check

After merge with `release/minor`:

```bash
git ls-remote --tags https://ghe.coxautoinc.com/ETS-CloudAutomation/alks-go | grep v1.1.0
curl -s https://proxy.golang.org/github.com/!cox-!automotive/alks-go/@v/v1.1.0.info
```

Expected: the tag on GHE, and module proxy info for `v1.1.0` (SC-008).
