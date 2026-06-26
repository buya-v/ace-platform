output "replication_group_id" {
  description = "ID of the Redis replication group"
  value       = aws_elasticache_replication_group.this.id
}

output "configuration_endpoint_address" {
  description = "Cluster-mode configuration endpoint clients connect to"
  value       = aws_elasticache_replication_group.this.configuration_endpoint_address
}

output "security_group_id" {
  description = "Security group ID guarding Redis access"
  value       = aws_security_group.redis.id
}

output "port" {
  description = "Redis port"
  value       = aws_elasticache_replication_group.this.port
}
