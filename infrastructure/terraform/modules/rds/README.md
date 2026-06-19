# Module: rds

RDS PostgreSQL instance for the GarudaX platform, with per-tenant KMS isolation.

## Tenant scoping

- Storage and the AWS-managed master secret are encrypted with a **platform CMK** (`alias/<project>-<env>-rds`).
- For every `tenant_id` in `var.tenant_ids`, a dedicated **per-tenant CMK** is created (`alias/<project>-<env>-rds-<tenant_id>`) for encrypting tenant-scoped data such as logical backups, exports, and per-tenant snapshots.
- The master password never enters Terraform state — it is generated and rotated by AWS Secrets Manager (`manage_master_user_password`).

## Key inputs

| Variable | Description |
|---|---|
| `tenant_ids` | List of tenant identifiers; one CMK per tenant. Defaults to `["ace-commodities"]`. |
| `instance_class`, `allocated_storage`, `multi_az` | Instance sizing. |
| `vpc_id`, `data_subnet_ids`, `eks_node_sg_id` | Networking; only EKS nodes may reach port 5432. |

## Key outputs

`db_endpoint`, `db_instance_arn`, `master_user_secret_arn`, `kms_key_arn`, `tenant_kms_key_arns` (map of `tenant_id => CMK ARN`).
