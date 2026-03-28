REJECTED

# Review — T003: EKS Cluster + Istio Service Mesh

**Verdict:** REJECTED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: FAIL

The implementation is comprehensive and covers EKS cluster, OIDC/IRSA, 5 managed node groups, Karpenter autoscaling, Istio mTLS, NodeLocal DNSCache, namespaces, and Kustomize overlays. However, there is a correctness bug:

- **Duplicate YAML key in `deploy/helm/istio/values-istiod.yaml`**: `meshConfig.defaultConfig` appears twice (lines ~22 and ~27). YAML silently uses the last occurrence, so `holdApplicationUntilProxyStarts: true` is silently dropped, and only the `tracing` config survives. For a commodity exchange platform with STRICT mTLS, this means application containers can start before the Istio sidecar proxy is ready, causing connection failures on startup. This is a real data-loss-risk bug.

- **DestinationRule host mismatch in `deploy/k8s/base/istio-mtls.yaml`**: `host: "*.local"` should be `"*.svc.cluster.local"` to correctly match all in-mesh services. The current pattern may not match services as intended.

- **`node_groups` variable type** in `variables.tf` does not include `labels` or `taints` fields, but `local.default_node_groups` defines them and the merge logic in `local.node_groups` assumes they may be present. Users passing custom node groups will get Terraform type errors if they try to include labels/taints, or will silently lose the default labels/taints if they override an existing group key. The variable type should match the actual usage.

### Security: PASS

Strong security posture throughout:
- KMS envelope encryption for K8s secrets with key rotation enabled
- STRICT mTLS mesh-wide with TLS 1.3 minimum
- `REGISTRY_ONLY` outbound traffic policy (no egress to unregistered services)
- SQS SSE enabled for Karpenter interruption queue
- EBS volumes encrypted on Karpenter-provisioned nodes
- Private API endpoint by default (`cluster_endpoint_public_access = false`)
- SSM managed instance core for node access (no SSH keys)
- IRSA with properly scoped conditions (sub + aud)
- No hardcoded secrets or credentials
- Karpenter IAM policy uses `Resource: *` for EC2 describe/create actions, which is standard and expected for Karpenter's dynamic provisioning model

### Code Quality: PASS

- Terraform is well-structured: proper `depends_on` chains, `lifecycle` rules, `create_before_destroy` on security groups, `ignore_changes` on desired_size
- Consistent tagging with `local.common_tags`
- Partition-aware ARNs (`data.aws_partition.current.partition`) for GovCloud compatibility
- Kustomize base/overlay pattern is clean with appropriate per-environment scaling
- Helm values files include installation instructions as comments
- Handoff file is thorough with clear cross-references to downstream tasks (T005, T006)
- NodeLocal DNSCache manifest follows the upstream Kubernetes addon pattern

### Test Coverage: PASS

56 structural tests covering:
- Terraform module file existence and required resources
- Istio mTLS configuration (STRICT mode, TLS 1.3, REGISTRY_ONLY)
- Karpenter NodePool counts, EC2NodeClass, volume encryption
- NodeLocal DNSCache (DaemonSet, link-local IP, priority class, host networking)
- Namespace definitions and Istio injection labels
- Kustomize overlay structure

Tests are appropriate for this stage (no running cluster). They verify structural correctness of all manifests and Terraform resources. They do not catch the duplicate YAML key bug (a YAML linter would).

## Required Fixes

1. **Fix duplicate `defaultConfig` key in `deploy/helm/istio/values-istiod.yaml`**: Merge the two `defaultConfig` blocks into one:
   ```yaml
   meshConfig:
     defaultConfig:
       holdApplicationUntilProxyStarts: true
       tracing:
         sampling: 100.0
   ```

2. **Fix DestinationRule host in `deploy/k8s/base/istio-mtls.yaml`**: Change `host: "*.local"` to `host: "*.svc.cluster.local"`.

3. **Fix `node_groups` variable type in `variables.tf`**: Add optional `labels` and `taints` fields to the variable type, or document that overrides only support the four declared fields.

## Suggestions (non-blocking)

- Consider adding a YAML lint step to the test suite (`yamllint` or `yq` validation) to catch duplicate key issues automatically.
- The `meshMTLS.minProtocolVersion` setting under `meshConfig` (line 24 of istiod values) is not a standard Istio field at that path. The correct path is `meshConfig.meshMTLS.minProtocolVersion` only in older Istio versions; in current versions, use `meshConfig.defaultConfig.tls.minProtocolVersion` or set it via PeerAuthentication. Verify against your target Istio version.
- The staging overlay is identical to base (just imports it). This is fine as a placeholder but consider documenting the intent.
- `tracing.sampling: 100.0` is appropriate for dev/staging but should be reduced in production overlays to avoid excessive trace volume.
