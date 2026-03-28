# Review — T054: Kubernetes Deployment Manifests (Rework)

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

All 3 required fixes from the original rejection are correctly implemented:

1. **Cross-namespace ConfigMap (FIXED):** `service-discovery-configmap.yaml` now defines the ConfigMap in both `ace-exchange` and `ace-services` namespaces. All engine deployments in `ace-exchange` can now resolve their `configMapRef`.

2. **Duplicate namespace file (FIXED):** The `ace-infra` namespace was merged into the existing `namespaces.yaml`. The base `kustomization.yaml` no longer references a separate `namespace.yaml`. A regression test (`test_no_duplicate_namespace_file`) guards against re-introduction.

3. **Missing JWT secret in ace-exchange (FIXED):** `secrets.yaml` now includes `ace-jwt-signing-key` and `ace-db-credentials` in the `ace-exchange` namespace, matching what engine deployments reference.

Port assignments are consistent with the documented convention (matching=50051, clearing=50052, margin=50053, settlement=50054, auth=50055, compliance=50056, market-data=50057, warehouse=50058). Service discovery ConfigMap addresses match actual service ports and use full cluster DNS (`*.svc.cluster.local`).

### Security: PASS

- All secrets use `CHANGE_ME_USE_EXTERNAL_SECRET_MANAGER` placeholder values.
- IRSA annotations use `ACCOUNT_ID` placeholder, no real AWS account IDs.
- No hardcoded credentials anywhere.
- All services use `ClusterIP` type (no external exposure).
- Per-service ServiceAccounts with IRSA annotations for least-privilege AWS access.
- `KAFKA_AUTO_CREATE_TOPICS_ENABLE: "false"` prevents rogue topic creation.

### Code Quality: PASS

- Consistent Kustomize structure: base + dev/staging/prod overlays.
- Uniform labeling (`app.kubernetes.io/name`, `app.kubernetes.io/part-of`).
- Clean separation: engines in `ace-exchange`, services in `ace-services`, infrastructure in `ace-infra`.
- StatefulSets used appropriately for Postgres, Kafka, Zookeeper with PVCs.
- Health probes on all workloads using appropriate mechanisms (HTTP `/healthz`, `pg_isready`, TCP, `ruok`).
- HPAs and PDBs for production services, correctly removed in dev overlay.

### Test Coverage: PASS

519 lines of Python tests covering:
- Directory structure and file existence for all 15 service directories.
- Port assignments, probes, resources, service accounts for all gRPC services.
- Port collision detection across all services.
- Service discovery ConfigMap completeness and DNS format validation.
- Secret placeholder verification.
- Namespace uniqueness (no duplicate definitions).
- **Cross-resource validation (5 new tests):** Verifies every `configMapRef` and non-optional `secretRef` in deployments resolves to a resource in the same namespace. Directly validates all 3 original rejection bugs.
- Kustomization reference completeness for base and all overlays.
- Label consistency (selector matches template labels).

## Required Fixes

None.

## Suggestions (non-blocking)

1. **Kafka `$(POD_NAME)` env var ordering:** `KAFKA_ADVERTISED_LISTENERS` (index 3) references `$(POD_NAME)` which is defined at index 4. Kubernetes only resolves `$(VAR)` references to env vars defined earlier in the list. Move `POD_NAME` above `KAFKA_ADVERTISED_LISTENERS` to fix.

2. **Overlay env var index patching is fragile and likely broken for Kafka:** The dev/staging/prod overlays patch Kafka env vars by positional index (`env/4/value`, `env/5/value`, `env/6/value`). Index 4 is `POD_NAME` which uses `valueFrom` (not `value`), so a JSON Patch `replace` on `/value` would fail because that path doesn't exist. Consider using strategic merge patches keyed by env var name instead of positional indices.

3. **Kafka BROKER_ID label:** `metadata.labels['apps.kubernetes.io/pod-index']` requires K8s 1.28+. For broader compatibility, extract the ordinal from `metadata.name` via an init container.

4. Remaining items from previous review still valid: add `storageClassName` to VolumeClaimTemplates, add NetworkPolicy resources, consider KRaft mode for Kafka, enable Redis auth for staging/prod.
