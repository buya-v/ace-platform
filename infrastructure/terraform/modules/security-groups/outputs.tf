output "alb_security_group_id" {
  description = "Security group ID for the public-facing ALB"
  value       = aws_security_group.alb.id
}

output "app_security_group_id" {
  description = "Security group ID for application workloads (ingress from ALB only)"
  value       = aws_security_group.app.id
}
