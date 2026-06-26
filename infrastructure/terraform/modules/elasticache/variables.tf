variable "project_name" {
  description = "Project identifier used in resource naming and tags"
  type        = string
}

variable "environment" {
  description = "Deployment environment: dev, staging, prod"
  type        = string
}

variable "node_type" {
  description = "ElastiCache Redis node type"
  type        = string
}

variable "num_shards" {
  description = "Number of node groups (shards) in the cluster-mode replication group"
  type        = number
}

variable "replicas_per_shard" {
  description = "Number of read replicas per shard"
  type        = number
}

variable "vpc_id" {
  description = "ID of the VPC the cluster is created in"
  type        = string
}

variable "data_subnet_ids" {
  description = "Private data subnet IDs (one per AZ) for the cache subnet group"
  type        = list(string)
}

variable "eks_node_sg_id" {
  description = "Security group ID of the EKS worker nodes permitted to reach Redis"
  type        = string
}

variable "tags" {
  description = "Common resource tags"
  type        = map(string)
  default     = {}
}
