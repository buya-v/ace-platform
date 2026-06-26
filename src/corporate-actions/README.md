# corporate-actions

**Module type: library (not a deployable service).**

`package corporateactions` models the corporate-actions state machine and the
financial calculations for dividends, rights issues, and splits. It is a pure,
zero-side-effect Go library: domain types, a lifecycle state machine
(`engine.go`), and per-event processing/value math (`process.go`). All money
math uses the shared `github.com/garudax-platform/decimal` type (wired via a
relative `replace` in `go.mod`).

## Why there is no `main.go` / `Dockerfile`

This module is intentionally **not** a runnable binary. Unlike the deployable
services (which expose `cmd/<name>/main.go` + a `Dockerfile`), corporate-actions
is consumed as a library by the service that will own corporate-action
processing. It is therefore deliberately **absent from every service list** —
Docker Compose, Kubernetes manifests, and CI build/deploy matrices — and must
not be added to them as a standalone service.

It builds and tests as a standalone module:

```sh
cd src/corporate-actions
go build ./...
go test ./...
```

## Roadmap

The engine is scheduled to be wired into a binary during **Phase 0.8
(mse-equities flagship build)**, where corporate actions become a first-class
equities concern (dividends, rights, splits with FRC reporting). At that point a
consuming service (e.g. an equities/corporate-actions service) will import this
package and provide the `cmd/.../main.go` entrypoint and `Dockerfile`. Until
then it remains a library with no entrypoint by design.
