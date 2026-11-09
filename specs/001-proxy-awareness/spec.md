# Feature Specification: System-proxy awareness for alks-go

**Feature Branch**: `feat/proxy-awareness`

**Created**: 2026-11-06

**Status**: Draft

**Input**: Problem spec "System-proxy awareness for the ALKS Terraform provider and alks-go" (Dave Barcelo, Ready for SDD, 2026-11-06), the alks-go rows of its section "SDD scope for alks-go": R1–R7, R8, platform section 4.2, R9–R12, R12a, R13–R15, R16, the alks-go part of R17, the alks-go part of R18, R23, the alks-go tests in its section 6, and its open items 1, 2 and 4. Tracked on Rally F325998: Placeholder: Lock down ALKS clients to Cox-only traffic.

**Scope boundary**: This spec covers the alks-go library only. Provider arguments, environment-variable names, the conflict rule, the provider's User-Agent string, provider documentation and the provider release belong to the separate terraform-provider-alks work, which consumes the tagged alks-go release this spec produces. Where a requirement here needs provider work, it is recorded as a dependency, not delivered.

## User Scenarios & Testing *(mandatory)*

Actors:
- **Engineer**: runs a tool built on alks-go (the ALKS Terraform provider, or another team's tool) from a Cox laptop or a CI runner.
- **Library consumer**: a developer whose Go code calls alks-go.
- **Maintainer**: releases alks-go and supports its consumers.

### User Story 1 - ALKS calls follow the system proxy with nothing to configure (Priority: P1)

An engineer on a Cox laptop whose Zscaler profile only routes traffic from proxy-aware applications runs a tool built on alks-go. Today the tool's ALKS calls go direct, from a non-Cox address, and the ALKS allowlist rejects them with an HTML 403 page. With this feature the library reads the operating system's proxy configuration, including the PAC file, and sends ALKS calls through the proxy the OS names, so the calls succeed with no environment variables set. (Problem spec R1, R2, 4.2.)

**Why this priority**: This is the failure being fixed. It was reproduced on two affected Windows laptops on 2026-11-06: direct and environment-only calls were blocked, and calls through the system proxy reached ALKS.

**Independent Test**: With a fake OS proxy source returning a PAC that names a local recording proxy for the ALKS host, a request made by a default client reaches the recording proxy, and a request to a host the PAC sends `DIRECT` does not.

**Acceptance Scenarios**:

1. **Given** no proxy environment variables and an OS PAC that returns `PROXY host:port` for the ALKS URL, **When** a default client makes an ALKS request, **Then** the request goes through `host:port`.
2. **Given** an OS PAC that proxies the ALKS host and returns `DIRECT` for another host, **When** requests are made to both, **Then** only the ALKS request uses the proxy.
3. **Given** no PAC but an OS static HTTPS proxy and an exception list that does not include the ALKS host, **When** an ALKS request is made, **Then** it goes through the static proxy.
4. **Given** an OS PAC that answers `PROXY a:9000; DIRECT` and nothing listening on `a:9000`, **When** an ALKS request is made, **Then** it fails with an error that names the proxy as unreachable, and it does not go direct.

---

### User Story 2 - Existing behaviour and explicit settings still win (Priority: P1)

An engineer or a CI runner already sets `HTTPS_PROXY`, uses `NO_PROXY`, or runs on Linux or in a container. Nothing changes for them. An engineer on an unusual setup can turn off OS discovery or force a proxy. (Problem spec R4, R5, R6, R13–R15.)

**Why this priority**: The fix must not break the many users and pipelines that work today. Equal in importance to Story 1.

**Independent Test**: The selection logic is exercised with every combination of environment variables, OS settings and modes, and with OS discovery failing, against expected decisions.

**Acceptance Scenarios**:

1. **Given** `HTTPS_PROXY` is set and the OS also has a proxy, **When** an ALKS request is made, **Then** `HTTPS_PROXY` is used, exactly as today.
2. **Given** `NO_PROXY` matches the ALKS host and the OS PAC names a proxy, **When** an ALKS request is made in the default mode, **Then** it goes direct.
3. **Given** a Linux or FreeBSD system with no proxy variables, **When** an ALKS request is made, **Then** it goes direct, exactly as today, and no desktop proxy settings are read.
4. **Given** the OS proxy settings or the PAC cannot be read or evaluated, **When** an ALKS request is made, **Then** the library falls back to the environment, then direct, without failing for that reason, and records the fallback at debug level.
5. **Given** a consumer selects the environment-only mode, **When** an ALKS request is made, **Then** only the standard proxy variables are consulted.
6. **Given** a consumer selects the no-proxy mode, **When** an ALKS request is made with `HTTPS_PROXY` set, **Then** it goes direct.
7. **Given** a consumer supplies an explicit proxy URL, **When** an ALKS request is made, **Then** that proxy is used, and `NO_PROXY` does not apply.

---

### User Story 3 - Library consumers configure the client in code, without breaking existing code (Priority: P2)

A library consumer, such as the Terraform provider or another team's tool, upgrades alks-go. Existing calls compile and behave as before, apart from the new default. The consumer can choose the proxy mode, an explicit proxy, a User-Agent, or supply their own HTTP client, and every one of those choices takes effect from the very first request the client makes. (Problem spec R9–R12, alks-go part of R17.)

**Why this priority**: The provider work depends on it, and at least one other consumer (`MAN-VehicleInformationRTC/actions-mapper`) imports the library today.

**Independent Test**: Existing constructor calls compile unchanged. A client built with a custom User-Agent sends that User-Agent on the request made during construction, verified with a local test server.

**Acceptance Scenarios**:

1. **Given** existing code that calls `NewClient`, `NewSTSClient` or `NewBearerTokenClient` with today's arguments, **When** it is compiled against the new version, **Then** it compiles without changes.
2. **Given** a client constructed with a User-Agent option, **When** the constructor makes its first ALKS request, **Then** that request carries the given User-Agent, not the default `alks-go`.
3. **Given** a client constructed with a proxy option, **When** the constructor makes its first ALKS request, **Then** that request uses the chosen proxy behaviour.
4. **Given** a consumer supplies their own HTTP client, **When** requests are made, **Then** the library uses it and does not override its proxy settings.

---

### User Story 4 - No credentials in logs (Priority: P2)

An engineer turns on debug logging to troubleshoot (for example `TF_LOG=DEBUG` in Terraform). Today alks-go writes every request in full to the log, authentication headers and body included. With this feature the log shows enough to troubleshoot, including the proxy decision, and never shows credentials. (Problem spec R12a, R16.)

**Why this priority**: A security defect found during this work; fixed in the same change because the same code is being modified.

**Independent Test**: Requests are made with each credential type while log output is captured; the capture contains no credential values and does contain the proxy-decision line.

**Acceptance Scenarios**:

1. **Given** a client using basic, STS or bearer credentials, **When** it makes a request, **Then** the log contains no password, access key, secret key, session token or bearer token, and no request body carrying them.
2. **Given** a client making its first request, **When** debug logging is captured, **Then** one line records the proxy mode, where the decision came from, and the proxy host, with any credentials in a proxy URL removed.

---

### User Story 5 - Tagged releases (Priority: P3)

A maintainer releases the change as a version that consumers can pin, and every later change is released the same way. Consumers pin `v1.1.0` instead of a commit hash. (Problem spec R23, alks-go constitution Principle VII.)

**Why this priority**: Needed before the provider can pin the change, but it does not change runtime behaviour.

**Independent Test**: A pull request without a release label fails the label check. Merging a labelled pull request produces the expected tag on the source repository, and the mirror carries it to the public module path.

**Acceptance Scenarios**:

1. **Given** the current master before this work, **When** it is tagged, **Then** the tag is `v1.0.0`.
2. **Given** a pull request with no release label, **When** its checks run, **Then** a check fails asking for `release/patch`, `release/minor`, `release/major` or `release/skip`.
3. **Given** this work merged with `release/minor`, **When** the release runs, **Then** `v1.1.0` is tagged on the source repository, appears at the public module path, and has a changelog entry.
4. **Given** a pull request labelled `release/skip`, **When** it merges, **Then** no tag is created.

---

### Edge Cases

- The PAC file is served from a local address that is temporarily down: discovery fails and the library falls back (Story 2, scenario 4). This is different from a proxy that was chosen but does not answer (Story 1, scenario 4), which is an error.
- The PAC script errors, times out, or takes a long time because it performs DNS lookups: evaluation is time-limited and treated as a discovery failure.
- The PAC answer is malformed, empty, or names an unsupported proxy type: treated as `DIRECT` for that request, and logged at debug level.
- The PAC answer's first entry is `DIRECT`: the request goes direct.
- A proxy URL contains credentials (`http://user:pass@host:port`): the credentials never appear in logs.
- `NO_PROXY` contains `*`, a domain with a leading dot, a CIDR range, or a port: matched by Go's standard rules.
- `localhost` and loopback targets: never proxied.
- Lowercase `https_proxy` / `no_proxy`: honoured as Go does today.
- The code runs inside a container on a laptop: no OS proxy is visible, so the behaviour is environment variables, else direct (problem spec R14). Consumers can pass an explicit proxy such as `http://host.docker.internal:9000`.
- Many requests in one run: the PAC is not re-evaluated for every request to the same host.

## Requirements *(mandatory)*

### Functional Requirements

Each requirement cites the problem-spec requirement it delivers.

**Proxy selection**

- **FR-001**: By default, the library MUST choose the proxy for each ALKS request in this order: the standard `HTTPS_PROXY` variable filtered by `NO_PROXY`, if set; otherwise the OS proxy configuration (the PAC result for the request URL, then the OS static proxy and its exception list); otherwise direct. (R1)
- **FR-002**: The PAC MUST be evaluated for the actual request URL, so per-host PAC rules are honoured. (R2)
- **FR-003**: When a PAC answer lists several entries, the library MUST use only the first. If that proxy cannot be reached, the request MUST fail with an error naming the proxy, and MUST NOT fall back to a later entry or to direct. If the first entry is `DIRECT`, the request MUST go direct. (R3)
- **FR-004**: `NO_PROXY` MUST exempt matching hosts from OS-discovered proxies as well as from environment proxies, using Go's standard matching rules. (R4)
- **FR-005**: A consumer that sets `HTTPS_PROXY` MUST get exactly today's behaviour. (R5)
- **FR-006**: If the OS proxy settings or the PAC cannot be read or evaluated within a bounded time, the library MUST fall back to the environment, then direct, MUST NOT fail the request for that reason, and MUST record the fallback at debug level. (R6)
- **FR-007**: TLS certificate verification MUST remain on and use the platform trust store, as today. (R7)
- **FR-008**: The change MUST affect only the library's own HTTP client for ALKS requests. (R8)
- **FR-009**: On macOS the OS proxy MUST be discovered without cgo, including PAC evaluation. On Windows it MUST be discovered from the WinHTTP / Internet Options settings, including PAC and automatic detection. On Linux, FreeBSD and any other platform the library MUST NOT read desktop proxy settings and MUST behave as today. (4.2, R13)

**Modes and options**

- **FR-010**: The library MUST offer three proxy modes: automatic (FR-001, the default), environment-only (standard variables only, today's behaviour), and none (always direct, ignoring every proxy setting including `HTTPS_PROXY`). (R1, R10; problem spec 4.3)
- **FR-011**: The library MUST accept an explicit proxy URL that is used for every ALKS request and to which `NO_PROXY` does not apply. Supplying an explicit proxy URL together with the none mode MUST be rejected as a configuration error. (problem spec 4.3)
- **FR-012**: The existing constructors MUST accept optional trailing settings for proxy mode, explicit proxy URL, User-Agent and a caller-supplied HTTP client, so existing calls compile unchanged. (R9, R12)
- **FR-013**: With no settings, the library MUST use the automatic mode. (R10)
- **FR-014**: All settings, including the User-Agent and the proxy, MUST apply to every request the client makes, including the request made during construction. (R11, alks-go part of R17)
- **FR-015**: When a caller supplies their own HTTP client, the library MUST use it as given and MUST NOT alter its proxy behaviour. (R9)
- **FR-016**: The library MUST NOT read any environment variable of its own. Consumers own their variable names and pass the results as settings. (alks-go constitution Principle V)
- **FR-017**: No exported identifier, constructor signature or behaviour other than the proxy default MAY change in a way that breaks existing callers. (R12)

**Logging**

- **FR-018**: The library MUST NOT write credentials (passwords, access keys, secret keys, session tokens, bearer tokens, credentials in URLs) or request bodies that carry them to logs or error messages. The current full-request dump MUST be removed or replaced with a redacted line. (R12a)
- **FR-019**: The library MUST record, once per client at debug level, the proxy mode, the source of the decision (explicit setting, environment, PAC, OS static setting, direct) and the proxy host, with credentials removed. (R16)

**Releases**

- **FR-020**: The repository MUST tag the current master as `v1.0.0` before this work merges. (R23)
- **FR-021**: The repository MUST have a release process driven by a label on the merged pull request (`release/patch`, `release/minor`, `release/major`, `release/skip`) that tags the source repository with the next semantic version; the existing mirror carries tags to the public module path. (R23)
- **FR-022**: Every pull request MUST be checked for a release label. (R23)
- **FR-023**: The repository MUST keep a changelog with an entry for every release. This work is released as `v1.1.0`. (R23)
- **FR-024**: The library's user-facing documentation (the README and the Cloud Platform Docs alks-go page) MUST describe the default, the three modes, the settings, and how a consumer opts out. (alks-go part of R18)

### Key Entities

- **Proxy mode**: automatic, environment-only or none; one per client; default automatic.
- **Proxy decision**: for one request, the proxy to use (or direct) and where the decision came from; recorded once per client in the debug log.
- **OS proxy configuration**: what the operating system reports: whether a PAC is configured and where, any static HTTPS proxy and its exceptions, and on Windows whether automatic detection is on.
- **PAC answer**: the ordered list returned by the PAC for a URL; only the first entry is used.
- **Release**: a semantic-version tag on the source repository, mirrored to the public module path, with a changelog entry.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On a machine where direct ALKS calls are blocked and the OS proxy is configured, 100% of ALKS requests from a default client reach ALKS with no proxy environment variables set (reproduces the 2026-11-06 laptop checks: today 403, with the feature reached ALKS).
- **SC-002**: Every existing constructor call in this repository's tests, and in the provider's current usage, compiles and passes unchanged.
- **SC-003**: For every combination in the selection test matrix (mode × environment variables × OS settings × discovery failure), the decision matches the expected result: 100% pass.
- **SC-004**: Across requests made with each credential type, captured log output contains zero credential values.
- **SC-005**: The request made during construction carries the configured User-Agent in 100% of constructions where one is supplied.
- **SC-006**: The library builds without cgo for every target the provider releases (Windows, Linux, macOS and FreeBSD on amd64, 386, arm and arm64).
- **SC-007**: Repeated requests to the same host within one client do not re-run the PAC; a client making 1,000 requests to one host evaluates the PAC at most once for that host.
- **SC-008**: `v1.1.0` resolves at the public module path, so a consumer can depend on `github.com/Cox-Automotive/alks-go v1.1.0`.

## Assumptions

- The problem spec's decisions are final for this work: the default, the three modes, first-PAC-entry-only, `NO_PROXY` on OS proxies, no environment reading in the library, Linux unchanged, and versioning from `v1.0.0`.
- The library's existing users are the Terraform provider and `actions-mapper`, both pinned to a 2023 commit; others may exist and are treated as if they do.
- The tests run without real ALKS, AWS, a network beyond loopback, or the developer's real OS proxy settings (constitution Principle III). OS discovery is replaceable in tests.
- The PAC engine, caching policy and evaluation time limit are chosen in the plan (problem spec open items 1 and 2).
- The provider-side work, which adds settings, environment variables, the User-Agent string, docs and the 2.9.0 release, depends on `v1.1.0` and is not part of this spec.
- The `actions-mapper` owners are told about the new default before `v1.1.0` is released (problem spec open item 4); this is a communication task, not code.
