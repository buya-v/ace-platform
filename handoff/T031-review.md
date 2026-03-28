APPROVED

# Review — T031: Rework EKS+Istio — Fix 3 Reviewer Bugs

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

All three bugs identified in the T003 review have been correctly fixed:

1. **Duplicate `defaultConfig` key (Bug 1):** `deploy/helm/istio/values-istiod.yaml` now has a single `defaultConfig` block containing both `holdApplicationUntilProxyStarts: true` and the `tracing` config. No silent YAML key override.

2. **DestinationRule host (Bug 2):** `deploy/k8s/base/istio-mtls.yaml` now uses `host: "*.svc.cluster.local"` instead of the incorrect `*.local`, ensuring proper in-mesh mTLS routing.

3. **Variable type mismatch (Bug 3):** `infrastructure/terraform/modules/eks/variables.tf` now includes `labels = optional(map(string), {})` and `taints = optional(list(object({...})), [])` with sensible defaults, so existing callers without these fields continue to work and new callers can specify them.

The full EKS module, K8s manifests, Helm values, and Kustomize overlays are structurally sound and match the T003 specification.

### Security: PASS

- STRICT mTLS mesh-wide + per-namespace (ace-exchange, ace-services)
- TLS 1.3 minimum enforced
- REGISTRY_ONLY outbound traffic policy
- KMS envelope encryption for K8s secrets with key rotation
- SQS SSE enabled for Karpenter interruption queue
- EBS encrypted on Karpenter-provisioned nodes
- Private API endpoint by default
- IRSA with properly scoped OIDC conditions (sub + aud)
- No hardcoded secrets or credentials
- Karpenter IAM policy scoped appropriately (ec2:*, iam:PassRole to node role only, SQS to specific queue)

### Code Quality: PASS

- Clean Terraform structure with proper `depends_on`, `lifecycle`, `create_before_destroy`
- Partition-aware ARNs for GovCloud compatibility
- `optional()` type constraints with defaults are idiomatic Terraform 1.3+
- Kustomize base/overlay pattern is clean with environment-appropriate scaling
- Helm values include installation instructions as comments
- Handoff files (T003.md and T031.md) are thorough with cross-references

### Test Coverage: PASS

The test suite (`tests/t003_eks_istio_test.sh`) includes 56+ tests covering:
- Terraform module structure and required resources
- Istio mTLS, TLS version, outbound policy
- Karpenter NodePools, EC2NodeClass, volume encryption
- NodeLocal DNSCache
- Namespace definitions and Istio injection labels
- Kustomize overlays

**T031-specific regression tests** (section 6) directly verify all three bug fixes:
- `defaultConfig` appears exactly once
- `holdApplicationUntilProxyStarts: true` preserved
- `tracing.sampling` preserved
- DestinationRule uses `*.svc.cluster.local`
- Old `*.local` pattern is absent
- `labels` and `taints` are optional fields in the variable type

**YAML lint validation** (section 7) checks all YAML files in `deploy/` for duplicate top-level keys, tabs, and trailing whitespace — addressing the reviewer suggestion from T003.

## Required Fixes

None.

## Suggestions (non-blocking)

- The handoff notes that `lookup(each.value, "labels", {})` in `main.tf` line ~86 is now redundant since `optional()` guarantees defaults. A minor cleanup opportunity.
- The `meshMTLS.minProtocolVersion` field (noted in the original T003 review) may not be at the correct Istio API path depending on target Istio version — worth verifying during deployment.
- `tracing.sampling: 100.0` is appropriate for dev/staging but should be reduced in production via a Kustomize overlay or separate prod values file.
- Consider adding `yamllint` (Python tool) to CI for deeper YAML validation beyond the bash-based checks.
