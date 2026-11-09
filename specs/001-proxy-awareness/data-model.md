# Data Model: System-proxy awareness for alks-go

In-memory types only; nothing is persisted.

## ProxyMode (exported)

| Value | Meaning |
|---|---|
| `ProxyAuto` (default, zero value) | `HTTPS_PROXY` (filtered by `NO_PROXY`) → OS PAC for the URL → OS static proxy → direct |
| `ProxyEnvironment` | `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY` only (Go's `http.ProxyFromEnvironment`), today's behaviour |
| `ProxyNone` | always direct; every proxy setting ignored |

The zero value is `ProxyAuto`, so a client built without options gets the default (FR-013).

## clientConfig (unexported)

Built from the options before any request is made.

| Field | Type | Default | Rule |
|---|---|---|---|
| `proxyMode` | `ProxyMode` | `ProxyAuto` | must be one of the three values; unknown → constructor error |
| `proxyURL` | `*url.URL` | nil | if set: must parse and have scheme `http` or `https` and a host; with `ProxyNone` → constructor error (FR-011) |
| `userAgent` | `string` | `"alks-go"` | empty string ignored (keeps default), as `SetUserAgent` does today |
| `httpClient` | `*http.Client` | nil | if set: used as-is; proxy settings ignored (FR-015) |

## OSProxyConfig (unexported)

Snapshot of the OS settings, read once per client (research R-5).

| Field | Meaning |
|---|---|
| `pacURL` | PAC location if PAC is enabled, else empty |
| `autoDetect` | Windows automatic detection (WPAD) enabled |
| `staticHTTPS` | static HTTPS proxy `host:port`, else empty |
| `exceptions` | static-proxy exception list (macOS `ExceptionsList`, Windows bypass list) |
| `excludeSimpleHostnames` | macOS: bypass hosts without a dot |
| `err` | the discovery error, if reading failed (a discovery failure, FR-006) |

## osProxySource (unexported interface)

| Method | Returns |
|---|---|
| `config()` | `OSProxyConfig` |
| `pacProxyFor(url *url.URL)` | the **first** PAC entry for that URL (`"DIRECT"`, `"PROXY host:port"`, or empty), or a discovery error |

Implementations: darwin (scutil + PAC fetch + `internal/pac`), windows (go-ieproxy), other (nothing). Tests use a fake.

## ProxyDecision (unexported)

| Field | Values |
|---|---|
| `proxy` | `*url.URL` or nil (direct) |
| `source` | `explicit`, `env`, `pac`, `os-static`, `direct`, `none-mode`, `env-mode`, `custom-client` |
| `fallback` | discovery error text if the decision followed a discovery failure |

Cached per scheme+host+port for the client's lifetime. Logged once per client (FR-019), with `redactURL(proxy)`.

## Selection rules (auto mode)

1. If `HTTPS_PROXY`/`https_proxy` applies to the URL (Go's `httpproxy`, including `NO_PROXY`), use it: `source=env`.
2. Else, if `NO_PROXY` matches the URL host: direct, `source=direct`.
3. Else, if the OS has a PAC URL or auto-detect, take the PAC's first entry:
   - `PROXY h:p` → `http://h:p`, `source=pac`;
   - `DIRECT` → direct;
   - empty, malformed or unsupported → direct.
4. Else, if the OS has a static HTTPS proxy and the host is not in its exceptions → that proxy, `source=os-static`.
5. Else direct.

At any step, a discovery failure skips the OS steps and continues to direct, with `fallback` recorded.

## State

The client holds its `clientConfig`, its OS snapshot, and the decision cache (a mutex-protected map). There are no other state transitions.
