# alks-go #

alks-go is a Go client library for accessing the ALKS API.

**Documentation:** [pkg.go.dev](https://pkg.go.dev/github.com/Cox-Automotive/alks-go) · **Changes:** [CHANGELOG.md](CHANGELOG.md)

alks-go requires Go 1.22 or greater. Development happens at `ghe.coxautoinc.com/ETS-CloudAutomation/alks-go`; the module is published through the `github.com/Cox-Automotive/alks-go` mirror.

## Usage ##

```go
import "github.com/Cox-Automotive/alks-go"
```

Construct a new ALKS client, then use the various services on the client to
access different parts of the ALKS API. Please note that session creation requires username and password. IAM role CRUD operations can work with either username and password or an STS session.

*Username/Password Authentication*
```go
client, err := alks.NewClient("http://my.alks.url/rest", "username", "password", "my-acct", "my-role")

// create new STS
resp, err := client.CreateSession(2, false)

log.Printf("Session: %v ~~ %v ~~ %v", resp.AccessKey, resp.SecretKey, resp.SessionToken)
```

*STS Authentication* - Currently only used for IAM role CRUD
```go
client, err := alks.NewSTSClient("http://my.alks.url/rest", "accessKey", "secretKey", "sessionToken")

// create new role
resp, err := client.CreateIamRole("myRole", "Amazon EC2", false)

log.Printf("Role ARN: %v ~~ Role IP ARN: %v", resp.roleArn, resp.roleIPArn)
```

```go
client, err := alks.NewSTSClient("http://my.alks.url/rest", "accessKey", "secretKey", "sessionToken")

// create new trust role
resp, err := client.CreateIamTrustRole("myRole", "Cross Account", "arn:aws:iam::123456789123:role/test-role")

log.Printf("Role ARN: %v ~~ Role IP ARN: %v", resp.roleArn, resp.roleIPArn)
```

Some API methods don't require an account and role to be provided.
```go
client, err := alks.NewClient("http://my.alks.url/rest", "username", "password", "", "")

// list all available account/roles
resp, err := client.GetAccounts()

for _,acct := range resp.Accounts{
    log.Printf("Account %v Role %v IAM %v", acct.Account, acct.Role, acct.IamActive)
}
```

## Client options ##

Every constructor accepts optional trailing options. Existing calls work unchanged.

```go
client, err := alks.NewSTSClient(url, accessKey, secretKey, token,
	alks.WithUserAgent("my-tool/1.2.3"),       // also used for the constructor's own request
	alks.WithProxyMode(alks.ProxyAuto),        // the default
)
```

| Option | Effect |
|---|---|
| `WithProxyMode(mode)` | `ProxyAuto` (default), `ProxyEnvironment` or `ProxyNone`; see below |
| `WithProxyURL("http://host:port")` | Send every ALKS request through this proxy. `NO_PROXY` does not apply. Not allowed with `ProxyNone`. |
| `WithUserAgent(ua)` | User-Agent for every request, including the one `NewSTSClient` makes while it is constructed |
| `WithHTTPClient(c)` | Use your own `*http.Client` as is; the proxy options are then ignored |

## Proxies ##

By default (`ProxyAuto`) the client chooses a proxy for each ALKS request in this order:

1. `HTTPS_PROXY` (or `https_proxy`), unless `NO_PROXY` matches the host. This is the same as before v1.1.0.
2. The operating system's proxy configuration:
   - **macOS:** the settings shown by `scutil --proxy`. A PAC file is evaluated for the request URL; otherwise the static HTTPS proxy and its exception list are used.
   - **Windows:** WinHTTP / Internet Options, including PAC files and automatic detection.
   - **Linux, FreeBSD and others:** no OS step.
3. A direct connection.

Details:

- Only the **first** entry of a PAC answer is used. For `PROXY 127.0.0.1:9000; DIRECT`, if that proxy is unreachable the request fails with an error naming the proxy, rather than going direct.
- `NO_PROXY` exempts matching hosts from an OS-discovered proxy too. It uses Go's rules: `example.com` matches the domain and its subdomains, `.example.com` matches subdomains only, CIDR ranges and `*` are allowed.
- `localhost` and loopback addresses are never proxied.
- If the OS settings or the PAC file cannot be read, the client falls back to the environment, then direct.

| Mode | Behaviour |
|---|---|
| `ProxyAuto` | As above (default) |
| `ProxyEnvironment` | `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY` only, exactly as before v1.1.0 |
| `ProxyNone` | Always direct, ignoring every proxy setting |

alks-go reads no environment variables of its own; only Go's standard proxy variables apply. Tools built on alks-go map their own settings to these options.

**Zscaler.** On laptops whose Zscaler profile only routes proxy-aware apps, the OS reports a PAC file served from `http://127.0.0.1:9000`, and `ProxyAuto` follows it with nothing to configure. **Inside a container** the host's PAC is not visible. There, set `HTTPS_PROXY=http://host.docker.internal:9000`, or pass `WithProxyURL("http://host.docker.internal:9000")` (Docker Desktop). `127.0.0.1` inside a container is the container itself.

**Debugging.** The client logs one line per client with the proxy decision, and one line per request and response (method, path, status). It never logs headers, bodies or credentials.

## Development ##

```
go test ./...
```

On recent macOS, older Go toolchains build test binaries that abort with `dyld: missing LC_UUID load command`; use `GOTOOLCHAIN=go1.25.0 go test ./...`. Every release target must build with `CGO_ENABLED=0`. See `CLAUDE.md` and `.specify/memory/constitution.md` for the project's rules.

Releases: label the pull request `release/patch`, `release/minor`, `release/major` or `release/skip`, and add a `CHANGELOG.md` entry. Merging tags the next version.
