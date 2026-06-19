# Module: eks

EKS cluster + node groups + IRSA/OIDC + Karpenter for the GarudaX platform.

## Tenant scoping (`iam.tf`)

For every `tenant_id` in `var.tenant_ids`, this module provisions:

- **A per-tenant IRSA role** (`<cluster>-<tenant_id>-irsa`) assumable *only* by Kubernetes service accounts in that tenant's namespace (`tenant-<tenant_id>`). The trust policy scopes `sub` to `system:serviceaccount:tenant-<tenant_id>:*` via `StringLike`, so no other namespace can assume it.
- **A per-tenant KMS CMK** (`alias/<cluster>-<tenant_id>-tenant`) whose key policy grants encrypt/decrypt only to that tenant's IRSA role.
- **A per-tenant IAM policy** restricting the tenant role to *its own* CMK ARN.

This enforces the platform invariant that tenant resources are both identity- and cryptographically-isolated. Tenant ID is never optional.

## Key inputs

| Variable | Description |
|---|---|
| `tenant_ids` | List of tenant identifiers; one IRSA role + CMK per tenant. Defaults to `["ace-commodities"]`. |
| `cluster_version`, `node_groups` | Cluster + managed node group config. |
| `vpc_id`, `private_subnet_ids` | Networking. |

## Key outputs

`cluster_name`, `cluster_endpoint`, `oidc_provider_arn`, `node_security_group_id`, `tenant_irsa_role_arns` (map of `tenant_id => role ARN`), `tenant_kms_key_arns` (map of `tenant_id => CMK ARN`), `tenant_namespaces`.
