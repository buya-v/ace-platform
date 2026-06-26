APPROVED

# Review — R017: Repository hygiene

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

All of the worker's load-bearing claims were independently verified against the code on `main`:

- **vpc + security-groups were genuinely empty-but-referenced stubs.** `infrastructure/terraform/main.tf` instantiates `module "vpc"` and `module "security_groups"`, and `eks`/`rds`/`msk` consume `module.vpc.{vpc_id, private_app_subnet_ids, private_data_subnet_ids}` and `module.eks.node_security_group_id`. With the prior 0-byte `main.tf` files, `terraform apply` would have failed on unknown module outputs — so the composition was already broken. Implementing minimally-correct modules (rather than removing them and orphaning eks/rds/msk) is the right hygiene call. Every cross-module reference in the new code resolves:
  - `vpc/outputs.tf` exports exactly `vpc_id`, `private_app_subnet_ids`, `private_data_subnet_ids` that downstream modules read.
  - `elasticache` consumes `module.vpc.private_data_subnet_ids` and `module.eks.node_security_group_id` — both exist (`eks/outputs.tf:31` `node_security_group_id`).
  - `redis_node_type / redis_num_shards / redis_replicas_per_shard` are all declared in root `variables.tf` (lines 80–96).
- **CIDR math is sound.** `cidrsubnet(vpc_cidr, 4, i)` / `i+3` / `i+6` over 3 AZs carves a /16 into non-overlapping /20s (indices 0–8 of 16). NAT-per-AZ with per-AZ private route tables is a correct, resilient egress design.
- **ElastiCache config is valid** for cluster mode: `num_node_groups`/`replicas_per_node_group` with `automatic_failover_enabled = true` (required for cluster mode), at-rest CMK + transit encryption, `configuration_endpoint_address` output (populated only in cluster mode). No bug found.
- **iam/s3 removal is safe** — neither appears in `main.tf`, deploy, CI, or as a backing variable; pure orphan dirs.
- **V2–V5 migrations are empty blobs** (`e69de29`, confirmed zero-byte) and unreferenced by any runner/config (the only matches are handoff/history docs). DDL is V1 then V6+.
- **corporate-actions is correctly described as a library** — `package corporateactions` with no external importer. securities-service's `handlers_corporate_actions.go` uses its own `internal/store`/`internal/types`, not this module. `go build`/`go test` unaffected.

No logic errors or broken cross-references were found. The one limitation (below) is that the new HCL was never machine-validated.

### Security: PASS

- KMS CMK for Redis at-rest with `enable_key_rotation = true`; transit encryption enabled (TLS-only).
- Redis SG ingress restricted to the EKS node SG only; ALB SG opens 443 to `0.0.0.0/0` (expected for a public edge); app SG accepts 8080 only from the ALB SG. Sensible least-privilege wiring.
- No hardcoded secrets or credentials. `create_before_destroy` lifecycle on SGs is correct.

### Code Quality: PASS (with a scope note)

The HCL matches existing module conventions closely (header comment block, `local.identifier`/`common_tags = merge(var.tags, {...})`, per-module README with inputs/outputs tables). Documentation is thorough and the handoff cleanly classifies implement-vs-remove-vs-document with rationale per item.

Scope note (non-blocking): a 12-minute task titled "hygiene" expanded into ~330 lines of new production Terraform (three-tier VPC, NAT/EIP/route tables, KMS, a full ElastiCache replication group) and **wired a brand-new `module "elasticache"` into `main.tf`**, which makes the next `terraform apply` provision a Redis cluster that did not exist before. This is a deploy-affecting architectural addition made without a spec inside a cleanup task. It is defensible here because (a) the referenced modules were already broken stubs and (b) the `redis_*` variables + `dev.tfvars.example` already signaled intent, leaving them as dead variables was itself the hygiene smell. Acceptable, but it stretches the minimal-diff principle for hygiene work — see suggestions.

### Test Coverage: PASS

There is no Terraform test harness in this repo, so IaC changes carry no unit tests by convention — consistent with prior infra tasks. The only Go change (corporate-actions README) is documentation; `go build ./...` / `go test ./...` for that module are unaffected. Coverage expectations are met for the nature of the change.

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

1. **Gate the new HCL behind CI validation before any real apply.** `terraform` was unavailable in the worker env, so none of this module wiring has been machine-checked — exactly the cross-reference bug class that sank T003/T054. I verified the references by hand, but R014 (CI gate) must add `terraform fmt -check -recursive` + `terraform validate` for `infrastructure/terraform/` before this is trusted on a real `plan`/`apply`.
2. **Treat the `elasticache` wiring as a deploy-affecting change, not hygiene.** Calling it out in the PR/handoff (which the worker did) is good; ensure whoever runs the next `plan` knows a Redis replication group + KMS key will be newly created.
3. **Flyway missing-migration risk for V2–V5.** Deleting these is fine for fresh deploys (Flyway tolerates version gaps), but if these empty stubs were ever applied to a long-lived DB, a subsequent `flyway validate` will report "applied migration not resolved locally" unless `ignoreMissingMigrations` is set. No evidence they were applied in this pipeline, so low risk — but worth a one-line note in the deploy runbook.
4. **VPC CIDR assumption.** The `cidrsubnet(.., 4, ..)` layout assumes ≥ /20 (default /16 is fine). The README documents this; consider a `validation` block on `vpc_cidr` to fail fast if a smaller CIDR is ever supplied.
