APPROVED

# Review — R014: Re-enable CI build+test gate (REWORK)

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent
**Date:** 2026-06-26

---

## Summary

This rework resolves the single correctness defect that got the first attempt
rejected: a required `Validate` status check (`branch-protection.json` lists it on
`main` with `strict: true`) that could never report on a PR after `terraform.yml` was
guarded to `workflow_dispatch`-only — which would have bricked every merge. The worker
took the **preferred** remediation from the prior review: restore `pull_request` on
`terraform.yml`, keep the read-only `terraform-validate`/`terraform-plan` jobs running
on PRs, and guard only the mutating `terraform-apply-*` jobs to manual dispatch. All
three required checks now report on a normal PR.

I verified the required-check ⇄ trigger alignment against the actual files rather than
trusting the handoff.

---

## Evaluation

### Correctness: PASS

The bricking defect is genuinely fixed:

- `branch-protection.json` requires `["CI Gate", "Security Gate", "Validate"]` on
  `main` (`strict: true`).
- `terraform.yml` `terraform-validate` is `name: Validate` (`terraform.yml:26`). The
  diff restores `on: pull_request: [main, develop]` **and removes the old `paths:`
  filter**. The paths-filter removal is the load-bearing detail: a required check
  guarded by a paths filter would not report on PRs that don't touch
  `infrastructure/terraform/**`, re-creating the same permanently-pending bricking
  state. Without it, `Validate` reports on every PR.
- `security.yml` (`Security Gate`, line 130) runs on `pull_request`; `trivy-fs`/
  `trivy-config` don't set `exit-code`, so they upload SARIF without failing a clean
  PR. `Security Gate` reports on every PR.
- `ci.yml` (`CI Gate`) runs on `pull_request`; `ci-gate` is `if: always()`, needs
  `[build, test, terraform-validate]`, and correctly aggregates matrix results
  (`needs.<job>.result` is `failure` if any leg fails).

The mutating jobs are correctly re-guarded: `terraform-apply-{dev,staging,prod}` →
`if: github.event_name == 'workflow_dispatch'` (retaining `environment:` approval),
image push and EKS deploy → dispatch-only. The dead `deploy.yml`
`promote-staging`/`promote-prod` jobs (flagged non-blocking in the first review) are
removed, and no surviving job `needs:` them — no orphan reference introduced. The
`deploy.yml` `workflow_run` trigger on "Docker Build & Push" is gone, matching
docker-build.yml being dispatch-only.

Dynamic module discovery (`find src tests/reconciliation -name go.mod | jq -R -s`)
correctly picks up nested `shared/pkg/*` zero-dep modules + the reconciliation test
module; `jq` is preinstalled on `ubuntu-latest`. `test` runs `-race` without forcing
`CGO_ENABLED=0` (race needs cgo); `build` uses `CGO_ENABLED=0`; `GO_VERSION 1.25` +
`GOTOOLCHAIN: auto` matches the engine `go 1.25.0` directives.

### Security: PASS

No regression. `ci.yml` declares `permissions: contents: read`. Every cloud-mutating
path (terraform apply, EKS deploy, ECR push) is dispatch-only with `environment:`
approval — nothing auto-applies on merge. No hardcoded secrets; AWS via OIDC
`role-to-assume`. Removing the dead dev→staging→prod auto-promotion chain also closes
an unintended privilege-escalation-by-merge path. govulncheck was widened to scan all
modules via `find src -name go.mod`. Re-activating `security.yml` as an actually-running
required gate is a net improvement.

### Code Quality: PASS

Workflows are well-structured with inline rationale at each `on:` block and at the
gate/required-check boundary — exactly the context a maintainer needs to avoid
re-introducing the bricking bug. `lint` (`continue-on-error`, excluded from
`ci-gate.needs`) and `e2e-fullstack` (dispatch-only) are correctly non-gating. Naming
and YAML idiom match the repo.

### Test Coverage: PASS (structural)

CI configuration has no unit-testable surface; verification is structural
(required-check ⇄ trigger alignment, result aggregation, guard conditions), performed
by inspection above. The worker correctly flagged that workflows can't be executed
locally (no `terraform`/`act` in the worker env), making the first real PR run the
empirical confirmation step. Acceptable for an infra-enablement task — not a coverage
gap.

---

## Required Fixes (if REJECTED)

None — the prior review's required fix #1 was implemented via its preferred path.

## Suggestions (non-blocking)

1. **Redundant terraform validation.** `terraform.yml`'s `Validate` and `ci.yml`'s
   new `terraform-validate` both run `fmt -check -recursive` + `init -backend=false` +
   `validate` on the same dir, and both feed required checks. One is sufficient;
   consider dropping the `ci.yml` copy to avoid double-running and a double-failure on
   bad HCL.
2. **First-run risk on post-R017 HCL.** Because `Validate` gates on `terraform fmt
   -check`/`validate` and the R017-added `vpc`/`security-groups`/`elasticache` modules
   were never validated locally, the first PR may be blocked until that HCL is
   `fmt`-clean and valid. That is the gate working as intended, but note every merge —
   including pure-Go PRs — is now coupled to infra-HCL health. Worth a heads-up before
   the first PR.
3. **`terraform-plan` PR noise.** With the `paths:` filter removed, the matrix plan
   (dev/staging/prod) runs on every PR and goes red without `AWS_CI_ROLE_ARN`. It is
   non-gating, so it does not block merges, but a later infra task could re-scope it to
   run only when `infrastructure/**` changes.
4. **Unpinned `aquasecurity/trivy-action@master`** (pre-existing in `security.yml`) is
   a supply-chain risk now that the workflow is active; pin to a release tag/SHA.
5. **Apply branch protection.** `branch-protection.json` is inert data — it must be
   `PUT` to the repo (API `/repos/{owner}/{repo}/branches/main/protection` or Settings →
   Branches) for the gate to enforce. Noted in the handoff; surfaced here so it isn't
   missed at merge time.
