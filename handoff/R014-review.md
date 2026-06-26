REJECTED

# Review — R014: Re-enable CI build+test gate

**Verdict:** REJECTED
**Reviewer:** AI Reviewer Agent
**Date:** 2026-06-26

---

## Summary

This is a strong, well-reasoned change that is one surgical fix away from approval. The
new `ci.yml` build+test gate is correct, the dynamic module discovery is a genuine
improvement over the old one-level `src/*/` loop (which missed the nested
`src/shared/pkg/*` modules), the Go 1.25 + `GOTOOLCHAIN: auto` bump matches the engine
module directives, and guarding `deploy`/`docker-build`/`terraform` to `workflow_dispatch`
is the security-conscious call. The handoff is thorough and honest.

It is rejected for **one correctness defect**: guarding the *entire* `terraform.yml` to
manual dispatch makes the `Validate` status check — which `branch-protection.json` lists
as a **required** check on `main` (`strict: true`) — impossible to satisfy on a PR. Since
the whole point of this task is the merge gate, an artifact that would brick merges when
applied has to be reconciled before approval.

---

## Evaluation

### Correctness: FAIL

The core gate is correct, but the terraform/branch-protection interaction is broken:

- `terraform.yml` `terraform-validate` job is named `Validate` (`terraform.yml:26`).
- `branch-protection.json` requires `"Validate"` (alongside `"CI Gate"`, `"Security Gate"`)
  as a `required_status_checks.contexts` entry on `main`, with `strict: true`.
- R014 changed `terraform.yml`'s triggers to `workflow_dispatch` **only** (removed the
  `pull_request`/`push` triggers). The `Validate` check therefore **never runs on a PR**.
- A required status check that never reports leaves every PR in a permanently-pending
  ("Expected") state and blocks the merge. Applying `branch-protection.json` as-is after
  this change would block all merges to `main` — the exact opposite of the task goal.

The worker's handoff asserts `branch-protection.json` is "already present and correct;
left as-is" but did not analyze that its own guarding change invalidates the `Validate`
context. This is in-scope: the conflict is a direct consequence of the worker's design
choice to guard the whole terraform workflow rather than just the mutating `apply` jobs.

Note `deploy.yml`'s `promote-staging`/`promote-prod` jobs are now dead under
dispatch-only (their `if:` requires `github.event.workflow_run.*` context that can no
longer occur). Harmless, but it confirms the triggers weren't fully re-examined against
the jobs' run conditions. (Non-blocking — see Suggestions.)

Everything else in the gate is correct: `lint`/`build`/`test`/`ci-gate` jobs present,
`pull_request` trigger present, `concurrency` set, `ci-gate` keys off `build`+`test`
results, coverage emitted non-gating (correctly avoids the old false-fail-below-60%
behavior), and `test` runs `-race` without forcing `CGO_ENABLED=0` (so the race detector
actually works), while `build` uses `CGO_ENABLED=0`. `jq` is preinstalled on
`ubuntu-latest`, so `discover` is fine.

### Security: PASS

- Cloud-mutating workflows (`deploy`, `docker-build`, `terraform`) are guarded to manual
  `workflow_dispatch`, so no auto image-push / EKS deploy / `terraform apply` on merge.
  This is the right posture and aligns with the "build+test gate, not a deploy" intent.
- No hardcoded secrets; AWS auth uses OIDC `role-to-assume`; `permissions:` are scoped
  (`contents: read` on CI). Deploy/apply jobs retain `environment:` approval gates.
- `GITHUB_TOKEN` is the only token used by gitleaks (auto-provided).

### Code Quality: PASS

- Clear, well-commented workflow; explanatory header distinguishes gate-vs-deploy.
- Dynamic `discover` matrix is cleaner and more correct than the previous hardcoded loop.
- Lint is non-gating (`continue-on-error`, excluded from `ci-gate`) — a reasonable choice;
  dropping the `golangci-lint v1.57` pin (predates go1.25) for `go vet` is justified.

### Test Coverage: PASS

- The acceptance contract is `tests/cicd/test_workflows.py`, which parameterizes over all
  active `*.yml` and adds per-workflow assertions. The change makes all five workflows
  active `.yml`, satisfying the file-existence, structure, security-practice, CI-job,
  security-job, terraform/docker/deploy-job, and branch-protection assertions. The handoff
  claims 20→0 failures; the workflow structures I inspected are consistent with that
  (all jobs have `runs-on`+`steps`; checkout pinned `@v4`; OIDC role-to-assume; `on:`/`True`
  parsing handled by the tests).
- No new tests were required for an infra-enablement task; the existing contract suite is
  the right validation surface.

---

## Required Fixes (must address to flip to APPROVED)

1. **Reconcile the `Validate` required check with terraform being dispatch-only.** Pick one:
   - **Preferred:** keep the read-only `terraform-validate` (and `terraform-plan`) jobs
     running on `pull_request` (they mutate nothing), and guard only the mutating
     `terraform-apply-*` jobs (they already have `environment:` approval). This keeps the
     `Validate` required check satisfiable on every PR while still preventing auto-apply.
   - **Alternative:** if terraform must stay fully manual, remove `"Validate"` from
     `branch-protection.json` `rules.main.required_status_checks.contexts` so the required
     set is only checks that reliably run on every PR (`CI Gate`, `Security Gate`).
   Whichever path, ensure the required-check set in `branch-protection.json` exactly equals
   the set of checks that report on a normal PR — otherwise applying it blocks all merges.

---

## Suggestions (non-blocking)

- **Dead promotion jobs in `deploy.yml`:** under `workflow_dispatch`-only, `promote-staging`
  and `promote-prod` can never trigger (their `if:` needs `workflow_run` context). Either
  drop them or convert their gating to a dispatch-driven promotion input, so the file
  reflects what can actually run.
- **`security.yml` is now a required `Security Gate` on every PR.** Confirm it runs green on
  a PR that touches no infra — notably the `trivy-config` IaC scan over `infrastructure/`
  (the `security-gate` job fails if `trivy-config` fails). A flaky/failing security scan
  would block merges just like the `Validate` issue above.
- **Unpinned `aquasecurity/trivy-action@master`** (pre-existing in `security.yml`) is a
  supply-chain risk; pin to a release tag/SHA. Not introduced by R014, but now active.
- **Apply branch protection:** as the handoff notes, `branch-protection.json` is data only;
  it must be `PUT` to the repo for the gate to actually block. Worth wiring into R015.
- **Frontend out of the gate:** correct to defer until the R013-class fixture refresh lands,
  per the handoff — add it as a non-blocking job first so it doesn't red-wall merges.
