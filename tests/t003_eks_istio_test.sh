#!/usr/bin/env bash
# Tests for T003: EKS Cluster + Istio Service Mesh
# Validates Terraform module structure, K8s manifest correctness, and Helm values.
# Run: bash tests/t003_eks_istio_test.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PASS=0
FAIL=0
ERRORS=""

pass() { PASS=$((PASS + 1)); echo "  ✓ $1"; }
fail() { FAIL=$((FAIL + 1)); ERRORS="${ERRORS}\n  ✗ $1"; echo "  ✗ $1"; }

echo "=== T003 EKS + Istio Test Suite ==="
echo ""

# ---------------------------------------------------------------------------
# 1. Terraform EKS Module Structure
# ---------------------------------------------------------------------------
echo "--- Terraform EKS Module ---"

for f in main.tf variables.tf outputs.tf; do
  if [ -s "$REPO_ROOT/infrastructure/terraform/modules/eks/$f" ]; then
    pass "eks/$f exists and is non-empty"
  else
    fail "eks/$f is missing or empty"
  fi
done

# Verify required resources exist in main.tf
EKS_MAIN="$REPO_ROOT/infrastructure/terraform/modules/eks/main.tf"

for resource in \
  'resource "aws_eks_cluster"' \
  'resource "aws_iam_openid_connect_provider"' \
  'resource "aws_eks_node_group"' \
  'resource "aws_eks_addon" "vpc_cni"' \
  'resource "aws_eks_addon" "coredns"' \
  'resource "aws_eks_addon" "kube_proxy"' \
  'resource "aws_iam_role" "karpenter_controller"' \
  'resource "aws_kms_key"' \
  'resource "aws_security_group" "cluster"' \
  'resource "aws_security_group" "node"' \
  'resource "aws_sqs_queue" "karpenter_interruption"' \
  'resource "aws_iam_instance_profile" "karpenter_node"'; do
  if grep -q "$resource" "$EKS_MAIN"; then
    pass "EKS module contains $resource"
  else
    fail "EKS module missing $resource"
  fi
done

# Verify 5 default node groups
NODE_GROUP_COUNT=$(grep -c '"garudax-platform/node-role"' "$EKS_MAIN" || true)
if [ "$NODE_GROUP_COUNT" -ge 5 ]; then
  pass "5 node groups defined (system, exchange-core, app-general, monitoring, data-pipeline)"
else
  fail "Expected 5 node groups, found references to $NODE_GROUP_COUNT"
fi

# Verify IRSA OIDC provider
if grep -q 'aws_iam_openid_connect_provider' "$EKS_MAIN"; then
  pass "OIDC provider for IRSA is configured"
else
  fail "OIDC provider for IRSA is missing"
fi

# Verify VPC CNI prefix delegation
if grep -q 'ENABLE_PREFIX_DELEGATION' "$EKS_MAIN"; then
  pass "VPC CNI prefix delegation enabled"
else
  fail "VPC CNI prefix delegation not configured"
fi

# Verify EKS secrets encryption
if grep -q 'encryption_config' "$EKS_MAIN"; then
  pass "EKS secrets encryption configured"
else
  fail "EKS secrets encryption missing"
fi

# Verify outputs
EKS_OUTPUTS="$REPO_ROOT/infrastructure/terraform/modules/eks/outputs.tf"
for output in \
  'cluster_name' \
  'cluster_endpoint' \
  'oidc_provider_arn' \
  'node_security_group_id' \
  'karpenter_controller_role_arn' \
  'karpenter_instance_profile_name' \
  'node_group_names'; do
  if grep -q "output \"$output\"" "$EKS_OUTPUTS"; then
    pass "Output '$output' defined"
  else
    fail "Output '$output' missing"
  fi
done

echo ""

# ---------------------------------------------------------------------------
# 2. Istio Configuration
# ---------------------------------------------------------------------------
echo "--- Istio Configuration ---"

ISTIO_MTLS="$REPO_ROOT/deploy/k8s/base/istio-mtls.yaml"
if [ -s "$ISTIO_MTLS" ]; then
  pass "istio-mtls.yaml exists"
else
  fail "istio-mtls.yaml missing or empty"
fi

# Verify STRICT mTLS
STRICT_COUNT=$(grep -c 'mode: STRICT' "$ISTIO_MTLS" || true)
if [ "$STRICT_COUNT" -ge 1 ]; then
  pass "STRICT mTLS mode configured ($STRICT_COUNT policies)"
else
  fail "STRICT mTLS mode not found"
fi

# Verify PeerAuthentication resource
if grep -q 'kind: PeerAuthentication' "$ISTIO_MTLS"; then
  pass "PeerAuthentication resource defined"
else
  fail "PeerAuthentication resource missing"
fi

# Verify mesh-wide scope (istio-system namespace)
if grep -q 'namespace: istio-system' "$ISTIO_MTLS"; then
  pass "Mesh-wide mTLS in istio-system namespace"
else
  fail "Mesh-wide mTLS scope missing"
fi

# Verify Istiod Helm values
ISTIOD_VALUES="$REPO_ROOT/deploy/helm/istio/values-istiod.yaml"
if [ -s "$ISTIOD_VALUES" ]; then
  pass "Istiod Helm values exist"
else
  fail "Istiod Helm values missing"
fi

if grep -q 'REGISTRY_ONLY' "$ISTIOD_VALUES"; then
  pass "Outbound traffic policy set to REGISTRY_ONLY"
else
  fail "Outbound traffic policy not restricted"
fi

if grep -q 'TLSV1_3' "$ISTIOD_VALUES"; then
  pass "Minimum TLS version set to 1.3"
else
  fail "Minimum TLS version not configured"
fi

# Verify gateway values
if [ -s "$REPO_ROOT/deploy/helm/istio/values-gateway.yaml" ]; then
  pass "Istio gateway Helm values exist"
else
  fail "Istio gateway Helm values missing"
fi

echo ""

# ---------------------------------------------------------------------------
# 3. Karpenter Configuration
# ---------------------------------------------------------------------------
echo "--- Karpenter Configuration ---"

KARPENTER_VALUES="$REPO_ROOT/deploy/helm/karpenter/values.yaml"
if [ -s "$KARPENTER_VALUES" ]; then
  pass "Karpenter Helm values exist"
else
  fail "Karpenter Helm values missing"
fi

if grep -q 'interruptionQueue' "$KARPENTER_VALUES"; then
  pass "Karpenter interruption queue configured"
else
  fail "Karpenter interruption queue missing"
fi

KARPENTER_NP="$REPO_ROOT/deploy/k8s/base/karpenter-nodepools.yaml"
if [ -s "$KARPENTER_NP" ]; then
  pass "Karpenter NodePool manifests exist"
else
  fail "Karpenter NodePool manifests missing"
fi

NP_COUNT=$(grep -c 'kind: NodePool' "$KARPENTER_NP" || true)
if [ "$NP_COUNT" -ge 3 ]; then
  pass "At least 3 Karpenter NodePools defined ($NP_COUNT)"
else
  fail "Expected >= 3 NodePools, found $NP_COUNT"
fi

if grep -q 'kind: EC2NodeClass' "$KARPENTER_NP"; then
  pass "EC2NodeClass defined for Karpenter"
else
  fail "EC2NodeClass missing"
fi

if grep -q 'encrypted: true' "$KARPENTER_NP"; then
  pass "Karpenter node volumes encrypted"
else
  fail "Karpenter node volume encryption not configured"
fi

echo ""

# ---------------------------------------------------------------------------
# 4. NodeLocal DNSCache
# ---------------------------------------------------------------------------
echo "--- NodeLocal DNSCache ---"

DNSCACHE="$REPO_ROOT/deploy/k8s/base/nodelocal-dnscache.yaml"
if [ -s "$DNSCACHE" ]; then
  pass "NodeLocal DNSCache manifest exists"
else
  fail "NodeLocal DNSCache manifest missing"
fi

if grep -q 'kind: DaemonSet' "$DNSCACHE"; then
  pass "NodeLocal DNSCache deployed as DaemonSet"
else
  fail "NodeLocal DNSCache should be a DaemonSet"
fi

if grep -q '169.254.20.10' "$DNSCACHE"; then
  pass "NodeLocal DNSCache uses standard link-local IP"
else
  fail "NodeLocal DNSCache IP not configured"
fi

if grep -q 'system-node-critical' "$DNSCACHE"; then
  pass "NodeLocal DNSCache has system-node-critical priority"
else
  fail "NodeLocal DNSCache priority class missing"
fi

if grep -q 'hostNetwork: true' "$DNSCACHE"; then
  pass "NodeLocal DNSCache uses host networking"
else
  fail "NodeLocal DNSCache should use host networking"
fi

echo ""

# ---------------------------------------------------------------------------
# 5. Namespace Configuration
# ---------------------------------------------------------------------------
echo "--- Namespaces ---"

NS_FILE="$REPO_ROOT/deploy/k8s/base/namespaces.yaml"
for ns in istio-system istio-ingress karpenter garudax-exchange garudax-services garudax-monitoring; do
  if grep -q "name: $ns" "$NS_FILE"; then
    pass "Namespace '$ns' defined"
  else
    fail "Namespace '$ns' missing"
  fi
done

# Verify Istio injection labels
INJECTION_COUNT=$(grep -c 'istio-injection: enabled' "$NS_FILE" || true)
if [ "$INJECTION_COUNT" -ge 2 ]; then
  pass "Istio sidecar injection enabled on app namespaces ($INJECTION_COUNT)"
else
  fail "Istio sidecar injection labels missing"
fi

echo ""

# ---------------------------------------------------------------------------
# 6. Bug Fix Validations (T031)
# ---------------------------------------------------------------------------
echo "--- T031 Bug Fix Validations ---"

# Bug 1: Duplicate YAML key — meshConfig.defaultConfig must appear exactly once
DEFAULTCONFIG_COUNT=$(grep -c '  defaultConfig:' "$ISTIOD_VALUES" || true)
if [ "$DEFAULTCONFIG_COUNT" -eq 1 ]; then
  pass "meshConfig.defaultConfig appears exactly once (no duplicate key)"
else
  fail "meshConfig.defaultConfig appears $DEFAULTCONFIG_COUNT times (expected 1)"
fi

# Bug 1 continued: holdApplicationUntilProxyStarts must be preserved
if grep -q 'holdApplicationUntilProxyStarts: true' "$ISTIOD_VALUES"; then
  pass "holdApplicationUntilProxyStarts: true is preserved"
else
  fail "holdApplicationUntilProxyStarts: true is missing after merge"
fi

# Bug 1 continued: tracing config must be preserved
if grep -q 'sampling: 100.0' "$ISTIOD_VALUES"; then
  pass "tracing sampling config is preserved"
else
  fail "tracing sampling config is missing after merge"
fi

# Bug 2: DestinationRule host must use *.svc.cluster.local
if grep -q 'host: "\*.svc.cluster.local"' "$ISTIO_MTLS"; then
  pass "DestinationRule host uses *.svc.cluster.local"
else
  fail "DestinationRule host should be *.svc.cluster.local"
fi

# Bug 2 continued: Verify old incorrect pattern is NOT present
if grep -q 'host: "\*.local"' "$ISTIO_MTLS"; then
  fail "DestinationRule still has incorrect host *.local"
else
  pass "Old incorrect host pattern *.local is removed"
fi

# Bug 3: node_groups variable must include labels and taints fields
EKS_VARS="$REPO_ROOT/infrastructure/terraform/modules/eks/variables.tf"
if grep -q 'labels' "$EKS_VARS" && grep -q 'optional(map(string)' "$EKS_VARS"; then
  pass "node_groups variable includes optional labels field"
else
  fail "node_groups variable missing optional labels field"
fi

if grep -q 'taints' "$EKS_VARS" && grep -q 'optional(list(object' "$EKS_VARS"; then
  pass "node_groups variable includes optional taints field"
else
  fail "node_groups variable missing optional taints field"
fi

echo ""

# ---------------------------------------------------------------------------
# 7. YAML Lint Validation
# ---------------------------------------------------------------------------
echo "--- YAML Lint Validation ---"

yaml_lint_check() {
  local file="$1"
  local name="$2"

  if [ ! -f "$file" ]; then
    fail "YAML lint: $name not found"
    return
  fi

  # Check for duplicate top-level keys within each YAML document
  # Multi-document YAML files use --- separators, so check per-document
  local has_dupes=false
  local dupe_keys=""
  local doc_num=0
  local current_keys=""

  while IFS= read -r line; do
    if [ "$line" = "---" ] || [ -z "$line" ]; then
      # Check current document for dupes
      if [ -n "$current_keys" ]; then
        local doc_dupes
        doc_dupes=$(echo "$current_keys" | sort | uniq -d)
        if [ -n "$doc_dupes" ]; then
          has_dupes=true
          dupe_keys="$dupe_keys doc$doc_num:$doc_dupes"
        fi
      fi
      current_keys=""
      doc_num=$((doc_num + 1))
      continue
    fi
    # Capture top-level keys (non-space, non-comment start)
    if echo "$line" | grep -qE '^[a-zA-Z]'; then
      local key
      key=$(echo "$line" | sed 's/:.*//')
      current_keys="$current_keys
$key"
    fi
  done < "$file"
  # Check final document
  if [ -n "$current_keys" ]; then
    local doc_dupes
    doc_dupes=$(echo "$current_keys" | sort | uniq -d)
    if [ -n "$doc_dupes" ]; then
      has_dupes=true
      dupe_keys="$dupe_keys doc$doc_num:$doc_dupes"
    fi
  fi

  if [ "$has_dupes" = false ]; then
    pass "YAML lint: $name has no duplicate top-level keys"
  else
    fail "YAML lint: $name has duplicate top-level keys:$dupe_keys"
  fi

  # Check for tabs (YAML should use spaces only)
  if grep -qP '\t' "$file"; then
    fail "YAML lint: $name contains tab characters"
  else
    pass "YAML lint: $name uses spaces only (no tabs)"
  fi

  # Check for trailing whitespace
  if grep -qE ' +$' "$file"; then
    fail "YAML lint: $name has trailing whitespace"
  else
    pass "YAML lint: $name has no trailing whitespace"
  fi
}

# Lint all YAML files in deploy/
while IFS= read -r yaml_file; do
  rel_path="${yaml_file#$REPO_ROOT/}"
  yaml_lint_check "$yaml_file" "$rel_path"
done < <(find "$REPO_ROOT/deploy" -name '*.yaml' -type f | sort)

echo ""

# ---------------------------------------------------------------------------
# 8. Kustomization Structure
# ---------------------------------------------------------------------------
echo "--- Kustomization ---"

for overlay in dev staging prod; do
  if [ -s "$REPO_ROOT/deploy/k8s/overlays/$overlay/kustomization.yaml" ]; then
    pass "Kustomize overlay '$overlay' exists"
  else
    fail "Kustomize overlay '$overlay' missing"
  fi
done

if [ -s "$REPO_ROOT/deploy/k8s/base/kustomization.yaml" ]; then
  pass "Kustomize base kustomization.yaml exists"
else
  fail "Kustomize base kustomization.yaml missing"
fi

echo ""

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo "=== Results ==="
echo "  Passed: $PASS"
echo "  Failed: $FAIL"

if [ "$FAIL" -gt 0 ]; then
  echo ""
  echo "Failures:"
  echo -e "$ERRORS"
  exit 1
fi

echo ""
echo "All tests passed."
exit 0
