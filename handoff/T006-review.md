APPROVED

# Review — T006: CI/CD Pipeline

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The worker delivered all artifacts requested in the task description:
- **Build, test, lint** — `ci.yml` with service-aware change detection, matrix strategy, lint via golangci-lint, race-enabled tests with coverage threshold, and a CI gate job.
- **Security scan** — `security.yml` with CodeQL, Trivy (filesystem + IaC config), govulncheck, Gitleaks, and a security gate job. Weekly scheduled run included.
- **Container image push** — `docker-build.yml` with ECR push, Buildx caching, Trivy image scan on built images.
- **Terraform plan/apply** — `terraform.yml` with validate, plan-on-PR (with PR comments), and sequential apply (dev -> staging -> prod) gated by GitHub Environments.
- **Deployment** — `deploy.yml` with dev -> staging -> prod promotion chain, automatic rollback on failure, manual dispatch support.
- **Branch protection** — `branch-protection.json` with appropriate rules for main and develop.

The change detection logic in `ci.yml` is sound: it checks for `go.mod` or `.go` files before including a service, and uses diff-based detection on PRs vs build-all on push. The CI gate job correctly uses `if: always()` to report even when upstream jobs are skipped.

Minor note: the `ci.yml` change detection uses `origin/${{ github.base_ref }}` which is correct for PRs with `fetch-depth: 0`.

### Security: PASS

- **OIDC authentication** — All AWS credential steps use `role-to-assume` with OIDC (`id-token: write` permission), no static access keys anywhere.
- **No hardcoded secrets** — All sensitive values reference `${{ secrets.* }}`.
- **Minimal permissions** — Each workflow declares only the permissions it needs (e.g., `contents: read`, `security-events: write`).
- **Pinned action versions** — All actions use `@v4`/`@v5`/`@v3` version tags.
- **Image scanning** — Trivy scans built images with `exit-code: 1` on CRITICAL/HIGH, preventing vulnerable images from being pushed.
- **Secret detection** — Gitleaks with full history scan (`fetch-depth: 0`).

One observation: actions are pinned to major version tags (e.g., `@v4`) rather than commit SHAs. This is a common practice and acceptable, though SHA pinning would be more secure against tag hijacking. Non-blocking.

### Code Quality: PASS

- Workflows are well-organized with clear naming and logical job structure.
- Concurrency groups prevent wasted CI runs on rapid pushes.
- The `deploy.yml` uses GitHub Environments for approval gates, which is the correct pattern for promotion chains.
- Terraform workflow correctly uses workspaces matching the T002 convention (dev/staging/prod).
- The `branch-protection.json` is a useful declarative reference, though it requires manual application.
- Handoff file is thorough with clear follow-up items for downstream tasks.

The `deploy.yml` `promote-staging` and `promote-prod` jobs use `kubectl set image` directly (no kustomize), which is a simpler fallback compared to the main deploy job's kustomize-first approach. This inconsistency is minor and noted in the handoff as needing kustomize overlays to be created.

### Test Coverage: PASS

69 tests across 8 test classes covering:
- **Structural validation** — all workflows have name, triggers, jobs, runs-on, steps.
- **Security practices** — pinned actions, no plaintext secrets, OIDC auth enforcement.
- **Per-workflow specifics** — CI (lint/test/build/gate jobs, PR triggers, concurrency), Security (CodeQL, Trivy, secret detection, scheduled runs), Terraform (validate/plan/apply, all environments, sequential apply, environment gates), Docker (buildx, Trivy scan), Deploy (environment gate, manual trigger, promotion chain).
- **Branch protection** — file exists, valid JSON, PR reviews required, status checks enforced, force push disabled.

Tests use `yaml.safe_load` (not `yaml.load`), which is the correct security practice.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **SHA-pinned actions** — Consider pinning third-party actions (especially `aquasecurity/trivy-action@master` and `gitleaks/gitleaks-action@v2`) to commit SHAs to mitigate supply chain risks. The `@master` pin on trivy-action is particularly loose.
2. **govulncheck flag order** — In `security.yml`, `govulncheck ./... -C "$dir"` has the `-C` flag after the package pattern. While govulncheck may accept this, the conventional order is flags before arguments. Verify this works as intended.
3. **Deploy health check** — The `verify deployment health` step in `deploy.yml` uses `--field-selector=status.phase!=Running,status.phase!=Succeeded` which may miss pods still in `Pending` phase during rollout. Consider adding a brief wait or using `kubectl wait`.
