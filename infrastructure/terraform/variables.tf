variable "project_name" {
  description = "Project identifier used in resource naming and tags"
  type        = string
  default     = "garudax-platform"
}

variable "environment" {
  description = "Deployment environment: dev, staging, prod"
  type        = string
  validation {
    condition     = contains(["dev", "staging", "prod"], var.environment)
    error_message = "Environment must be dev, staging, or prod."
  }
}

variable "aws_region" {
  description = "Primary AWS region"
  type        = string
  default     = "ap-northeast-1"
}

variable "dr_region" {
  description = "Disaster recovery AWS region"
  type        = string
  default     = "ap-southeast-1"
}

variable "vpc_cidr" {
  description = "VPC CIDR block"
  type        = string
  default     = "10.0.0.0/16"
}

variable "eks_cluster_version" {
  description = "EKS Kubernetes version"
  type        = string
  default     = "1.28"
}

variable "eks_node_groups" {
  description = "EKS node group configurations"
  type = map(object({
    instance_type = string
    min_size      = number
    max_size      = number
    desired_size  = number
  }))
}

variable "rds_instance_class" {
  description = "RDS PostgreSQL instance class"
  type        = string
  default     = "db.r6g.2xlarge"
}

variable "rds_allocated_storage" {
  description = "RDS storage in GB"
  type        = number
  default     = 1000
}

variable "rds_multi_az" {
  description = "Enable Multi-AZ for RDS"
  type        = bool
  default     = true
}

variable "msk_instance_type" {
  description = "MSK Kafka broker instance type"
  type        = string
  default     = "kafka.m5.xlarge"
}

variable "msk_broker_count" {
  description = "Number of MSK brokers"
  type        = number
  default     = 3
}

variable "redis_node_type" {
  description = "ElastiCache Redis node type"
  type        = string
  default     = "cache.r6g.xlarge"
}

variable "redis_num_shards" {
  description = "Redis cluster shard count"
  type        = number
  default     = 6
}

variable "redis_replicas_per_shard" {
  description = "Redis replicas per shard"
  type        = number
  default     = 2
}

variable "tenant_ids" {
  description = "List of tenant (venue) identifiers provisioned on the platform. Each tenant gets isolated IRSA roles and KMS CMKs across EKS, RDS, and MSK. Platform invariant: tenant ID is never optional."
  type        = list(string)
  default     = ["ace-commodities"]

  validation {
    condition     = length(var.tenant_ids) > 0
    error_message = "At least one tenant_id must be provisioned."
  }
}

variable "tags" {
  description = "Common resource tags"
  type        = map(string)
  default     = {}
}
