APPROVED

# Review — R026: Fix Dockerfile build contexts so images build from current source

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The change correctly resolves the R015 deploy-tier P1: after R002 (`shared/pkg/types/decimal`) and R010 (`shared/pkg/tenant`), consuming modules use `replace ... => ../shared/...`, which points outside the old narrow `./src/<svc>` Docker context (`replacement directory ../shared/... does not exist`). Verified against the live tree:

- **Scope is exactly right.** `grep` for `replace ... => ../shared` returns 13 `go.mod` files. Of these, `corporate-actions` has **no Dockerfile** (library, R017) and is correctly left alone; the other 12 entries reduce to the **10 services** the diff widens (matching/clearing/margin/settlement engines, gateway, compliance, market-data, warehouse, securities, fix-gateway). `auth-service` (verified — no shared replace, network-fetched deps only) and `platform-service` have no shared replace and correctly keep the narrow `./src/<svc>` context. No shared-module consumer with a Dockerfile was missed.
- **Dockerfile pattern is sound.** `WORKDIR /build` → `COPY shared ./shared` → `COPY <svc> ./<svc>` → `WORKDIR /build/<svc>` → build. From `/build/<svc>`, the go.mod's `../shared/pkg/types/decimal` and `../shared/pkg/tenant` resolve to `/build/shared/pkg/...`. The shared sub-modules verifiably live at those paths.
- **The output-path detail is handled.** `/build/<svc>` is now a directory (the copied source), so the binary is correctly emitted to `/build/bin/<svc>` and that's what the runtime stage copies. This is the kind of subtle break that a careless port would miss.
- **compose changes match.** All 10 `build:` blocks switch to `context: ./src` + `dockerfile: <svc>/Dockerfile`; the 2 narrow services are untouched.
- **CI workflow is correctly made context-aware.** `detect-services` now emits `{service, context, dockerfile}` objects, deciding wide vs narrow by the same `replace => ../shared` grep (and defaulting to narrow when no `go.mod` exists, so frontend services stay correct). The matrix switches `service:` → `include:` (one job per object; `matrix.service` still resolves for the meta/trivy/build-arg steps), and the build step uses `context: ${{ matrix.context }}` + root-relative `file: ${{ matrix.dockerfile }}` — which is how `docker/build-push-action` resolves an explicit `file:` (relative to workspace root, not the context). Correct.

I could not independently run `docker build` in this review environment, so the "all 12 build PASS" claim rests on the worker's verification — but the resolution logic is verifiably correct from the module layout, and the change is mechanically consistent across all 10 Dockerfiles.

### Security: PASS

No secrets introduced. Each rewritten Dockerfile preserves the non-root `appuser`, static `CGO_ENABLED=0` build, and original `EXPOSE` ports. The new `src/.dockerignore` excludes `node_modules`, build artifacts, logs, and `.git`. Note that the wide `./src` context now ships every Go service's source into the *build context*, but each Dockerfile copies only `shared` + its own `<svc>` into the image, so no extra source lands in the final image — acceptable. `GOTOOLCHAIN=auto` matches documented R-series build behavior.

### Code Quality: PASS

Minimal, consistent diff. Every widened Dockerfile carries the same explanatory header citing the `replace`/context rationale and R026, which prevents a future narrowing regression. Follows the established shared-module/relative-`replace` conventions already in the repo. The CI `emit()` helper is readable and the wide/narrow decision is derived (grep) rather than hardcoded, so it stays correct as services are added/removed.

### Test Coverage: PASS

This is an infrastructure change (Dockerfiles, compose, CI matrix) with no unit-testable Go surface, so no application tests are expected — no Go source changed. The appropriate verification is the build itself; the worker reports real `docker compose build` for all 12 Go services, and live full-stack bring-up + e2e is correctly deferred to the R027 follow-up. Acceptable.

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

1. **Wire a non-pushing `docker build` (or `docker compose build`) check into the R014 CI gate** so Dockerfile-context / image-drift defects fail at PR time rather than being discovered days later at integration (the exact R015 root cause). The existing `docker-build.yml` push job is correctly deploy-only/dispatch-guarded; a build-only gate job is what belongs in CI.
2. **R025 is still required for a fully hands-off fresh bring-up** — R026 makes images buildable, but clean Postgres `initdb.d` init still needs zero-padded migration filenames and the `market_data.trades` V8/V16 dedupe. Worth restating in R027's preconditions.
3. **gha cache scope:** with multiple services now sharing the `./src` context, confirm `cache-from/to: type=gha` doesn't thrash across services (consider per-service `scope=`). Efficiency only, not correctness.
4. The wide context ships the whole `src/` tree per build; the `src/.dockerignore` keeps it ~1 MB today. If `src/shared` (go 1.24) grows, revisit a root builder / vendored-modules approach as the worker notes.
