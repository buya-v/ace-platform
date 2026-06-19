# Module: msk

Amazon MSK (Managed Streaming for Kafka) cluster for the GarudaX platform, with per-tenant KMS isolation.

## Tenant scoping

- At-rest broker storage is encrypted with a **platform CMK** (`alias/<project>-<env>-msk`); in-transit traffic is TLS-only (`client_broker = "TLS"`, `in_cluster = true`).
- For every `tenant_id` in `var.tenant_ids`, a dedicated **per-tenant CMK** is created (`alias/<project>-<env>-msk-<tenant_id>`) used to envelope-encrypt tenant-scoped topic payloads at the application layer.
- `auto.create.topics.enable=false` — topics are created explicitly per tenant.

## Key inputs

| Variable | Description |
|---|---|
| `tenant_ids` | List of tenant identifiers; one CMK per tenant. Defaults to `["ace-commodities"]`. |
| `instance_type`, `broker_count`, `broker_volume_size`, `kafka_version` | Broker sizing. `broker_count` must be a multiple of the number of `data_subnet_ids` (AZs). |
| `vpc_id`, `data_subnet_ids`, `eks_node_sg_id` | Networking; only EKS nodes may reach the brokers. |

## Key outputs

`cluster_arn`, `bootstrap_brokers_tls`, `zookeeper_connect_string`, `kms_key_arn`, `tenant_kms_key_arns` (map of `tenant_id => CMK ARN`).
