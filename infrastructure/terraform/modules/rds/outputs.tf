output "db_instance_arn" {
  description = "ARN of the RDS instance"
  value       = aws_db_instance.this.arn
}

output "db_endpoint" {
  description = "Connection endpoint (host:port) for the RDS instance"
  value       = aws_db_instance.this.endpoint
}

output "db_address" {
  description = "Hostname of the RDS instance"
  value       = aws_db_instance.this.address
}

output "db_port" {
  description = "Port of the RDS instance"
  value       = aws_db_instance.this.port
}

output "db_name" {
  description = "Initial database name"
  value       = aws_db_instance.this.db_name
}

output "master_user_secret_arn" {
  description = "ARN of the Secrets Manager secret holding the master credentials"
  value       = aws_db_instance.this.master_user_secret[0].secret_arn
}

output "security_group_id" {
  description = "Security group ID controlling access to RDS"
  value       = aws_security_group.rds.id
}

output "kms_key_arn" {
  description = "ARN of the platform CMK encrypting RDS storage"
  value       = aws_kms_key.rds.arn
}

output "tenant_kms_key_arns" {
  description = "Map of tenant_id => per-tenant RDS data CMK ARN"
  value       = { for t, k in aws_kms_key.tenant : t => k.arn }
}

output "tenant_kms_alias_names" {
  description = "Map of tenant_id => per-tenant RDS KMS alias name"
  value       = { for t, a in aws_kms_alias.tenant : t => a.name }
}
