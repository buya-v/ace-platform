variable "project_name" {
  description = "Project identifier used in resource naming"
  type        = string
}

variable "environment" {
  description = "Deployment environment: dev, staging, prod"
  type        = string
}

variable "instance_class" {
  description = "RDS PostgreSQL instance class"
  type        = string
  default     = "db.r6g.large"
}

variable "allocated_storage" {
  description = "Initial RDS storage in GB"
  type        = number
  default     = 100
}

variable "multi_az" {
  description = "Enable Multi-AZ deployment"
  type        = bool
  default     = false
}

variable "vpc_id" {
  description = "VPC ID where RDS is deployed"
  type        = string
}

variable "data_subnet_ids" {
  description = "Private data subnet IDs for the DB subnet group"
  type        = list(string)
}

variable "eks_node_sg_id" {
  description = "Security group ID of EKS nodes permitted to connect to the database"
  type        = string
}

variable "engine_version" {
  description = "PostgreSQL engine version"
  type        = string
  default     = "16.3"
}

variable "database_name" {
  description = "Initial database name"
  type        = string
  default     = "garudax"
}

variable "master_username" {
  description = "Master DB username (password is managed by AWS Secrets Manager)"
  type        = string
  default     = "garudax_admin"
}

variable "backup_retention_period" {
  description = "Number of days to retain automated backups"
  type        = number
  default     = 7
}

variable "tenant_ids" {
  description = "List of tenant (venue) identifiers. Each tenant receives a dedicated KMS CMK for tenant-isolated data (logical backups, exports, per-tenant snapshots)."
  type        = list(string)
  default     = ["ace-commodities"]

  validation {
    condition     = length(var.tenant_ids) == length(toset(var.tenant_ids))
    error_message = "tenant_ids must be unique."
  }
}

variable "tags" {
  description = "Common resource tags"
  type        = map(string)
  default     = {}
}
