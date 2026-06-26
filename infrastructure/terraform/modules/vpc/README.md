# Module: vpc

Platform network foundation: a three-tier VPC spread across the region's first
three Availability Zones.

| Tier           | Purpose                                  | Internet inbound |
| -------------- | ---------------------------------------- | ---------------- |
| `public`       | NAT gateways, future ALBs                | yes              |
| `private_app`  | EKS worker nodes / application workloads | no (NAT egress)  |
| `private_data` | RDS, MSK                                 | no (NAT egress)  |

Each AZ has its own NAT gateway and private route table so a single-AZ failure
cannot sever egress for the surviving zones.

## Inputs

| Name           | Type          | Description                               |
| -------------- | ------------- | ----------------------------------------- |
| `project_name` | `string`      | Naming/tag prefix                         |
| `environment`  | `string`      | `dev` \| `staging` \| `prod`              |
| `vpc_cidr`     | `string`      | VPC CIDR (>= /20 to fit the three tiers)  |
| `tags`         | `map(string)` | Common resource tags                      |

## Outputs

| Name                      | Description                              |
| ------------------------- | ---------------------------------------- |
| `vpc_id`                  | VPC ID                                   |
| `vpc_cidr`                | VPC CIDR block                           |
| `public_subnet_ids`       | Public subnet IDs (one per AZ)           |
| `private_app_subnet_ids`  | App-tier subnet IDs — consumed by `eks`  |
| `private_data_subnet_ids` | Data-tier subnet IDs — `rds` / `msk`     |
| `availability_zones`      | AZs the subnets span                     |
