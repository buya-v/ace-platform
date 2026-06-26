# Module: security-groups

Cross-cutting, platform-level security groups that are not owned by any single
data-plane tier. Per-service groups (RDS, MSK, EKS nodes) live with their own
modules so they can reference the consuming group directly.

| Group | Purpose                              | Ingress             |
| ----- | ------------------------------------ | ------------------- |
| `alb` | Public-facing application LBs        | HTTPS (443) from 0.0.0.0/0 |
| `app` | Application workloads behind the ALB | TCP 8080 from `alb` only   |

## Inputs

| Name           | Type          | Description                   |
| -------------- | ------------- | ---------------------------- |
| `project_name` | `string`      | Naming/tag prefix            |
| `environment`  | `string`      | `dev` \| `staging` \| `prod` |
| `vpc_id`       | `string`      | VPC to create the groups in  |
| `tags`         | `map(string)` | Common resource tags         |

## Outputs

| Name                    | Description                          |
| ----------------------- | ------------------------------------ |
| `alb_security_group_id` | ID of the public ALB security group  |
| `app_security_group_id` | ID of the application security group |
