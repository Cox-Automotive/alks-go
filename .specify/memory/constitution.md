<!--
Sync Impact Report
- Version: 1.0.0 → 1.0.1 (PATCH, wording; review of the setup PR)
- Principle I: caveat that a trailing variadic parameter changes the function's type
- Principle IV: names both former dumps (requests and responses), now removed
- Version history: template → 1.0.0 (initial constitution)
- Principles added: I–VII
- Templates: plan-template.md "Constitution Check" gates derive from the principles below; no template edits needed
- Follow-up: ratify through review of the feat/spec-kit-setup PR; add the label-driven release workflow that Principle VII requires; cut v1.0.0 on the current master
-->

# alks-go Constitution

alks-go is the Go client library for the ALKS API. Its main consumer is the ALKS Terraform provider (`ETS-CloudAutomation/terraform-provider-alks`), and at least one other team uses it. `CLAUDE.md` loads this file with an `@` import, so edits here apply to every Claude Code session as well as to spec-kit.

## Core Principles

### I. Backward-compatible public API

Consumers pin alks-go to a commit and upgrade on their own schedule, and not all of them are known. Exported functions, types, fields and constructor signatures MUST NOT change in a way that breaks existing callers. New behaviour is added through new functions, new fields, or trailing variadic options on existing constructors. A trailing variadic parameter keeps every direct call compiling, but it changes the function's type, so code that stores a constructor in a variable or field of the old function type would break: before adding one, check the known consumers and note it in `CHANGELOG.md`. Any unavoidable break needs a major version and an explicit migration note.

### II. Pure Go, no cgo

The provider is built with `CGO_ENABLED=0` and cross-compiled for Windows, Linux, macOS and FreeBSD (amd64, 386, arm, arm64) from a Linux runner. Every change, including every new dependency, MUST build and behave correctly under `CGO_ENABLED=0` for all of those targets. Platform-specific code uses Go build tags or file suffixes and MUST compile on the other platforms.

### III. Tested behaviour, no live services in tests

Every behaviour change ships with tests using the standard `testing` package, `net/http/httptest` or `testutils`. Tests MUST NOT call real ALKS, AWS or the network beyond the loopback interface, and MUST NOT depend on the developer's OS proxy settings. OS-dependent logic sits behind an interface that tests can fake. `go test ./...` passes on every PR.

### IV. No secrets in logs or errors

Credentials (passwords, access keys, secret keys, session tokens, bearer tokens) and credentials embedded in URLs MUST NOT appear in log output, error messages or test output. Redact before logging. Never dump whole requests or responses: until v1.1.0 `NewRequest` dumped every request (auth headers included) and `decodeBody` every response (session credentials included); both were removed.

### V. Configuration through code, not the environment

The library reads no environment variables of its own. Callers configure it through constructor options or setters. The only environment variables that affect it are the standard ones Go itself honours (`HTTPS_PROXY`, `HTTP_PROXY`, `NO_PROXY`). Consumers such as the provider own their environment-variable names and pass the results in.

### VI. Few, vendored, pure-Go dependencies

Dependencies are kept to a minimum, vendored (`vendor/`), pure Go (Principle II), actively maintained, and under a licence compatible with the repository's. Adding one requires a stated reason in the plan.

### VII. Semantic versioning with tags

Releases are annotated `vMAJOR.MINOR.PATCH` tags on GHE `master`; the mirror workflow copies them to github.com, where consumers resolve the module. Versioning starts at `v1.0.0` for the code as it stood before the first tagged change, because Principle I already promises a stable API. New behaviour is a MINOR release, a fix is a PATCH. A breaking change would need `v2`, which under Go's rules also means a new module path ending in `/v2`; Principle I exists to avoid that. As in the provider repository, the release is driven by a label on the merged PR (`release/patch`, `release/minor`, `release/major`, or `release/skip` for no release), and every release gets a `CHANGELOG.md` entry. Consumers, including the provider, pin tags, not pseudo-versions.

## Delivery

- **Source of truth:** `ghe.coxautoinc.com/ETS-CloudAutomation/alks-go`. The module path stays `github.com/Cox-Automotive/alks-go`. A workflow mirrors `master` and tags to github.com on every push, and consumers resolve the module from there. Never commit to the github.com mirror.
- **Changes reach the provider** only after it bumps its pin and re-vendors. A spec that needs a provider change says so and names the provider-side work.
- **Formatting:** code is `gofmt`-clean. Exported identifiers have doc comments.

## Governance

This constitution supersedes conflicting guidance elsewhere in the repository. Operational guidance (commands, layout, known issues) lives in `CLAUDE.md`, which loads this constitution. `CLAUDE.md` must not contradict it; where they conflict, this constitution wins and `CLAUDE.md` is corrected. Amendments are made by pull request that edits this file, bumps the version (MAJOR for removing or redefining a principle, MINOR for adding one, PATCH for wording), and updates the Sync Impact Report. Plans produced by spec-kit MUST pass the Constitution Check against these principles, or record a justified exception in the plan's complexity tracking.

**Version**: 1.0.1 | **Ratified**: 2026-11-06 | **Last Amended**: 2026-11-06
