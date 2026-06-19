variable "project_name" {
  description = "Project identifier used in resource naming"
  type        = string
}

variable "environment" {
  description = "Deployment environment: dev, staging, prod"
  type        = string
}

variable "instance_type" {
  description = "MSK Kafka broker instance type"
  type        = string
  default     = "kafka.m5.large"
}

variable "broker_count" {
  description = "Number of MSK broker nodes (must be a multiple of the number of client subnets / AZs)"
  type        = number
  default     = 3
}

variable "vpc_id" {
  description = "VPC ID where MSK brokers are deployed"
  type        = string
}

variable "data_subnet_ids" {
  description = "Private data subnet IDs (one per AZ) for broker placement"
  type        = list(string)
}

variable "eks_node_sg_id" {
  description = "Security group ID of EKS nodes permitted to connect to the brokers"
  type        = string
}

variable "kafka_version" {
  description = "Apache Kafka version"
  type        = string
  default     = "3.6.0"
}

variable "broker_volume_size" {
  description = "EBS volume size (GB) per broker"
  type        = number
  default     = 100
}

variable "tenant_ids" {
  description = "List of tenant (venue) identifiers. Each tenant receives a dedicated KMS CMK for encrypting tenant-scoped topic data. Tenant ID is never optional on the platform."
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
