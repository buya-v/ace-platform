output "vpc_id" {
  description = "ID of the platform VPC"
  value       = aws_vpc.this.id
}

output "vpc_cidr" {
  description = "CIDR block of the platform VPC"
  value       = aws_vpc.this.cidr_block
}

output "public_subnet_ids" {
  description = "Public subnet IDs (one per AZ)"
  value       = aws_subnet.public[*].id
}

output "private_app_subnet_ids" {
  description = "Private application-tier subnet IDs (one per AZ) — EKS worker nodes"
  value       = aws_subnet.private_app[*].id
}

output "private_data_subnet_ids" {
  description = "Private data-tier subnet IDs (one per AZ) — RDS / MSK"
  value       = aws_subnet.private_data[*].id
}

output "availability_zones" {
  description = "Availability Zones the subnets are spread across"
  value       = local.azs
}
