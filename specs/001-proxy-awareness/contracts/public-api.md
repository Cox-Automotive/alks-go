# Contract: alks-go public API added by this feature

This is the exported surface after the change. Everything not listed is unchanged (Constitution I).

## Constructors (signatures extended, existing calls still compile)

```go
func NewClient(url, username, password, account, role string, opts ...ClientOption) (*Client, error)
func NewSTSClient(url, accessKey, secretKey, token string, opts ...ClientOption) (*Client, error)
func NewBearerTokenClient(url, bearerToken, account, role string, opts ...ClientOption) (*Client, error)
```

- **Before any request:** options are applied before the constructor makes a request. Today only `NewSTSClient` makes one (`GET /rest/loginRoles/id/me`).
- **Invalid options** are returned as an error from the constructor. Examples are `WithProxyURL` combined with `ProxyNone`, a malformed proxy URL, or an unknown mode.
- **`NewSTSClient`** keeps today's behaviour of ignoring a failed login-role lookup.

## Options

```go
type ClientOption func(*clientConfig)

type ProxyMode int

const (
	ProxyAuto        ProxyMode = iota // default
	ProxyEnvironment                  // HTTPS_PROXY / HTTP_PROXY / NO_PROXY only (behaviour before v1.1.0)
	ProxyNone                         // always direct
)

func WithProxyMode(mode ProxyMode) ClientOption
func WithProxyURL(proxyURL string) ClientOption   // http or https URL; NO_PROXY not applied
func WithUserAgent(userAgent string) ClientOption // applies from the first request; empty = keep default
func WithHTTPClient(c *http.Client) ClientOption  // used as-is; proxy options ignored
```

`(ProxyMode).String()` returns `"auto"`, `"environment"` or `"none"`, so consumers can map their own configuration values.

## Existing methods (unchanged)

`SetUserAgent` keeps working and still affects later requests. `WithUserAgent` is the way to set the User-Agent for the constructor's own request.

## Behaviour changes a consumer will notice

| Before (pseudo-versions up to `v1.0.0`) | After (`v1.1.0`) |
|---|---|
| Proxy from `HTTPS_PROXY`/`NO_PROXY` only | Also the OS proxy and PAC on macOS and Windows, when no `HTTPS_PROXY` applies |
| The request made inside `NewSTSClient` always sent User-Agent `alks-go` | It sends the `WithUserAgent` value |
| Every request logged in full, credentials included | One redacted line per request, plus one proxy-decision line per client |
| `go 1.16` | `go 1.22` minimum |

To restore the old proxy behaviour exactly, pass `WithProxyMode(ProxyEnvironment)`.
