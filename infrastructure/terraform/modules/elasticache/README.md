# Module: elasticache

Cluster-mode (sharded) ElastiCache for Redis replication group, reachable only
from the EKS worker nodes. At-rest encryption uses a platform CMK; all client
traffic is TLS-only.

Cache-layer tenant isolation is logical (per-tenant keyspace prefixing in the
application) rather than per-CMK, because a single replication group serves all
venues.

## Inputs

| Name                 | Type           | Description                               |
| -------------------- | -------------- | ----------------------------------------- |
| `project_name`       | `string`       | Naming/tag prefix                         |
| `environment`        | `string`       | `dev` \| `staging` \| `prod`              |
| `node_type`          | `string`       | Redis node type (e.g. `cache.r6g.large`)  |
| `num_shards`         | `number`       | Node groups (shards)                      |
| `replicas_per_shard` | `number`       | Read replicas per shard                   |
| `vpc_id`             | `string`       | VPC to create the cluster in              |
| `data_subnet_ids`    | `list(string)` | Private data subnets for the subnet group |
| `eks_node_sg_id`     | `string`       | EKS node SG permitted to reach Redis      |
| `tags`               | `map(string)`  | Common resource tags                      |

## Outputs

| Name                             | Description                         |
| -------------------------------- | ----------------------------------- |
| `replication_group_id`           | Redis replication group ID          |
| `configuration_endpoint_address` | Cluster-mode configuration endpoint |
| `security_group_id`              | SG guarding Redis access            |
| `port`                           | Redis port (6379)                   |
