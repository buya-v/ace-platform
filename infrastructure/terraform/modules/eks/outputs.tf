output "cluster_name" {
  description = "EKS cluster name"
  value       = aws_eks_cluster.this.name
}

output "cluster_endpoint" {
  description = "EKS cluster API endpoint"
  value       = aws_eks_cluster.this.endpoint
}

output "cluster_certificate_authority_data" {
  description = "Base64-encoded certificate data for cluster CA"
  value       = aws_eks_cluster.this.certificate_authority[0].data
}

output "cluster_arn" {
  description = "ARN of the EKS cluster"
  value       = aws_eks_cluster.this.arn
}

output "cluster_version" {
  description = "Kubernetes version of the cluster"
  value       = aws_eks_cluster.this.version
}

output "cluster_security_group_id" {
  description = "Security group ID of the EKS cluster control plane"
  value       = aws_security_group.cluster.id
}

output "node_security_group_id" {
  description = "Security group ID of the EKS node group"
  value       = aws_security_group.node.id
}

output "node_role_arn" {
  description = "ARN of the IAM role for EKS nodes"
  value       = aws_iam_role.node.arn
}

output "oidc_provider_arn" {
  description = "ARN of the OIDC provider for IRSA"
  value       = aws_iam_openid_connect_provider.eks.arn
}

output "oidc_provider_url" {
  description = "URL of the OIDC provider (without https://)"
  value       = replace(aws_eks_cluster.this.identity[0].oidc[0].issuer, "https://", "")
}

output "karpenter_controller_role_arn" {
  description = "ARN of the Karpenter controller IRSA role"
  value       = aws_iam_role.karpenter_controller.arn
}

output "karpenter_instance_profile_name" {
  description = "Instance profile name for Karpenter-provisioned nodes"
  value       = aws_iam_instance_profile.karpenter_node.name
}

output "karpenter_interruption_queue_name" {
  description = "SQS queue name for Karpenter spot interruption handling"
  value       = aws_sqs_queue.karpenter_interruption.name
}

output "node_group_names" {
  description = "Names of all EKS managed node groups"
  value       = { for k, v in aws_eks_node_group.this : k => v.node_group_name }
}

output "node_group_status" {
  description = "Status of all EKS managed node groups"
  value       = { for k, v in aws_eks_node_group.this : k => v.status }
}
