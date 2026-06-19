output "cluster_arn" {
  description = "ARN of the MSK cluster"
  value       = aws_msk_cluster.this.arn
}

output "cluster_name" {
  description = "Name of the MSK cluster"
  value       = aws_msk_cluster.this.cluster_name
}

output "bootstrap_brokers_tls" {
  description = "TLS bootstrap broker connection string"
  value       = aws_msk_cluster.this.bootstrap_brokers_tls
}

output "zookeeper_connect_string" {
  description = "Zookeeper connection string"
  value       = aws_msk_cluster.this.zookeeper_connect_string
}

output "security_group_id" {
  description = "Security group ID controlling access to the brokers"
  value       = aws_security_group.msk.id
}

output "kms_key_arn" {
  description = "ARN of the platform CMK encrypting MSK at-rest data"
  value       = aws_kms_key.msk.arn
}

output "tenant_kms_key_arns" {
  description = "Map of tenant_id => per-tenant MSK topic-data CMK ARN"
  value       = { for t, k in aws_kms_key.tenant : t => k.arn }
}

output "tenant_kms_alias_names" {
  description = "Map of tenant_id => per-tenant MSK KMS alias name"
  value       = { for t, a in aws_kms_alias.tenant : t => a.name }
}
