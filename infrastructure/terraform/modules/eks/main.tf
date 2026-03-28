###############################################################################
# EKS Cluster + Node Groups
# Deploys: cluster, OIDC provider (IRSA), managed node groups, VPC CNI addon,
#          Karpenter IRSA role, CoreDNS & kube-proxy addons.
###############################################################################

data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}
data "aws_region" "current" {}

locals {
  cluster_name = "${var.project_name}-${var.environment}"

  common_tags = merge(var.tags, {
    "kubernetes.io/cluster/${local.cluster_name}" = "owned"
    TaskID                                        = "T003"
  })

  # 5 node groups: system, exchange-core, app-general, monitoring, data-pipeline
  default_node_groups = {
    system = {
      instance_type = "m5.large"
      min_size      = 2
      max_size      = 4
      desired_size  = 2
      labels = {
        "ace-platform/node-role" = "system"
      }
      taints = []
    }
    exchange-core = {
      instance_type = "c5.2xlarge"
      min_size      = 2
      max_size      = 8
      desired_size  = 2
      labels = {
        "ace-platform/node-role" = "exchange-core"
      }
      taints = [{
        key    = "ace-platform/exchange-core"
        value  = "true"
        effect = "NO_SCHEDULE"
      }]
    }
    app-general = {
      instance_type = "m5.xlarge"
      min_size      = 2
      max_size      = 10
      desired_size  = 2
      labels = {
        "ace-platform/node-role" = "app-general"
      }
      taints = []
    }
    monitoring = {
      instance_type = "m5.xlarge"
      min_size      = 1
      max_size      = 3
      desired_size  = 1
      labels = {
        "ace-platform/node-role" = "monitoring"
      }
      taints = [{
        key    = "ace-platform/monitoring"
        value  = "true"
        effect = "NO_SCHEDULE"
      }]
    }
    data-pipeline = {
      instance_type = "r5.xlarge"
      min_size      = 1
      max_size      = 6
      desired_size  = 1
      labels = {
        "ace-platform/node-role" = "data-pipeline"
      }
      taints = [{
        key    = "ace-platform/data-pipeline"
        value  = "true"
        effect = "NO_SCHEDULE"
      }]
    }
  }

  # Merge user-provided overrides with defaults; user values win for matching keys
  node_groups = merge(local.default_node_groups, {
    for k, v in var.node_groups : k => merge(
      lookup(local.default_node_groups, k, {
        labels = {}
        taints = []
      }),
      v
    )
  })
}

# -----------------------------------------------------------------------------
# IAM Role — EKS Cluster
# -----------------------------------------------------------------------------
resource "aws_iam_role" "cluster" {
  name = "${local.cluster_name}-cluster-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = {
        Service = "eks.amazonaws.com"
      }
    }]
  })

  tags = local.common_tags
}

resource "aws_iam_role_policy_attachment" "cluster_AmazonEKSClusterPolicy" {
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/AmazonEKSClusterPolicy"
  role       = aws_iam_role.cluster.name
}

resource "aws_iam_role_policy_attachment" "cluster_AmazonEKSVPCResourceController" {
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/AmazonEKSVPCResourceController"
  role       = aws_iam_role.cluster.name
}

# -----------------------------------------------------------------------------
# EKS Cluster
# -----------------------------------------------------------------------------
resource "aws_eks_cluster" "this" {
  name     = local.cluster_name
  version  = var.cluster_version
  role_arn = aws_iam_role.cluster.arn

  vpc_config {
    subnet_ids              = var.private_subnet_ids
    endpoint_private_access = var.cluster_endpoint_private_access
    endpoint_public_access  = var.cluster_endpoint_public_access
    security_group_ids      = [aws_security_group.cluster.id]
  }

  enabled_cluster_log_types = var.cluster_log_types

  encryption_config {
    provider {
      key_arn = aws_kms_key.eks.arn
    }
    resources = ["secrets"]
  }

  tags = local.common_tags

  depends_on = [
    aws_iam_role_policy_attachment.cluster_AmazonEKSClusterPolicy,
    aws_iam_role_policy_attachment.cluster_AmazonEKSVPCResourceController,
  ]
}

# -----------------------------------------------------------------------------
# KMS Key — Envelope encryption for K8s secrets
# -----------------------------------------------------------------------------
resource "aws_kms_key" "eks" {
  description             = "EKS secret encryption key for ${local.cluster_name}"
  deletion_window_in_days = 7
  enable_key_rotation     = true
  tags                    = local.common_tags
}

resource "aws_kms_alias" "eks" {
  name          = "alias/${local.cluster_name}-eks"
  target_key_id = aws_kms_key.eks.key_id
}

# -----------------------------------------------------------------------------
# Security Group — Cluster control plane
# -----------------------------------------------------------------------------
resource "aws_security_group" "cluster" {
  name_prefix = "${local.cluster_name}-cluster-"
  description = "EKS cluster control plane security group"
  vpc_id      = var.vpc_id

  tags = merge(local.common_tags, {
    Name = "${local.cluster_name}-cluster-sg"
  })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_security_group_rule" "cluster_egress" {
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = aws_security_group.cluster.id
  description       = "Allow all outbound traffic"
}

# -----------------------------------------------------------------------------
# OIDC Provider — for IRSA (IAM Roles for Service Accounts)
# -----------------------------------------------------------------------------
data "tls_certificate" "eks" {
  url = aws_eks_cluster.this.identity[0].oidc[0].issuer
}

resource "aws_iam_openid_connect_provider" "eks" {
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = [data.tls_certificate.eks.certificates[0].sha1_fingerprint]
  url             = aws_eks_cluster.this.identity[0].oidc[0].issuer
  tags            = local.common_tags
}

# -----------------------------------------------------------------------------
# IAM Role — Node Group
# -----------------------------------------------------------------------------
resource "aws_iam_role" "node" {
  name = "${local.cluster_name}-node-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = {
        Service = "ec2.amazonaws.com"
      }
    }]
  })

  tags = local.common_tags
}

resource "aws_iam_role_policy_attachment" "node_AmazonEKSWorkerNodePolicy" {
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/AmazonEKSWorkerNodePolicy"
  role       = aws_iam_role.node.name
}

resource "aws_iam_role_policy_attachment" "node_AmazonEKS_CNI_Policy" {
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/AmazonEKS_CNI_Policy"
  role       = aws_iam_role.node.name
}

resource "aws_iam_role_policy_attachment" "node_AmazonEC2ContainerRegistryReadOnly" {
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
  role       = aws_iam_role.node.name
}

resource "aws_iam_role_policy_attachment" "node_AmazonSSMManagedInstanceCore" {
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/AmazonSSMManagedInstanceCore"
  role       = aws_iam_role.node.name
}

# -----------------------------------------------------------------------------
# Security Group — Node Group
# -----------------------------------------------------------------------------
resource "aws_security_group" "node" {
  name_prefix = "${local.cluster_name}-node-"
  description = "EKS node security group"
  vpc_id      = var.vpc_id

  tags = merge(local.common_tags, {
    Name                                          = "${local.cluster_name}-node-sg"
    "kubernetes.io/cluster/${local.cluster_name}"  = "owned"
  })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_security_group_rule" "node_ingress_self" {
  type                     = "ingress"
  from_port                = 0
  to_port                  = 65535
  protocol                 = "-1"
  source_security_group_id = aws_security_group.node.id
  security_group_id        = aws_security_group.node.id
  description              = "Node-to-node communication"
}

resource "aws_security_group_rule" "node_ingress_cluster" {
  type                     = "ingress"
  from_port                = 443
  to_port                  = 443
  protocol                 = "tcp"
  source_security_group_id = aws_security_group.cluster.id
  security_group_id        = aws_security_group.node.id
  description              = "Cluster API to node kubelets"
}

resource "aws_security_group_rule" "node_ingress_cluster_kubelet" {
  type                     = "ingress"
  from_port                = 1025
  to_port                  = 65535
  protocol                 = "tcp"
  source_security_group_id = aws_security_group.cluster.id
  security_group_id        = aws_security_group.node.id
  description              = "Cluster API to node workloads"
}

resource "aws_security_group_rule" "node_egress" {
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = aws_security_group.node.id
  description       = "Allow all outbound traffic"
}

resource "aws_security_group_rule" "cluster_ingress_node" {
  type                     = "ingress"
  from_port                = 443
  to_port                  = 443
  protocol                 = "tcp"
  source_security_group_id = aws_security_group.node.id
  security_group_id        = aws_security_group.cluster.id
  description              = "Node to cluster API"
}

# -----------------------------------------------------------------------------
# EKS Managed Node Groups — 5 groups per spec
# -----------------------------------------------------------------------------
resource "aws_eks_node_group" "this" {
  for_each = local.node_groups

  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "${local.cluster_name}-${each.key}"
  node_role_arn   = aws_iam_role.node.arn
  subnet_ids      = var.private_subnet_ids
  instance_types  = [each.value.instance_type]

  scaling_config {
    min_size     = each.value.min_size
    max_size     = each.value.max_size
    desired_size = each.value.desired_size
  }

  labels = lookup(each.value, "labels", {})

  dynamic "taint" {
    for_each = lookup(each.value, "taints", [])
    content {
      key    = taint.value.key
      value  = taint.value.value
      effect = taint.value.effect
    }
  }

  update_config {
    max_unavailable_percentage = 25
  }

  tags = merge(local.common_tags, {
    Name      = "${local.cluster_name}-${each.key}"
    NodeGroup = each.key
  })

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonSSMManagedInstanceCore,
  ]

  lifecycle {
    ignore_changes = [scaling_config[0].desired_size]
  }
}

# -----------------------------------------------------------------------------
# EKS Addons — VPC CNI, CoreDNS, kube-proxy
# -----------------------------------------------------------------------------
resource "aws_eks_addon" "vpc_cni" {
  cluster_name                = aws_eks_cluster.this.name
  addon_name                  = "vpc-cni"
  resolve_conflicts_on_create = "OVERWRITE"
  resolve_conflicts_on_update = "OVERWRITE"

  configuration_values = jsonencode({
    env = {
      ENABLE_PREFIX_DELEGATION = "true"
      WARM_PREFIX_TARGET       = "1"
      ENABLE_POD_ENI           = "true"
    }
  })

  tags = local.common_tags
}

resource "aws_eks_addon" "coredns" {
  cluster_name                = aws_eks_cluster.this.name
  addon_name                  = "coredns"
  resolve_conflicts_on_create = "OVERWRITE"
  resolve_conflicts_on_update = "OVERWRITE"
  tags                        = local.common_tags

  depends_on = [aws_eks_node_group.this]
}

resource "aws_eks_addon" "kube_proxy" {
  cluster_name                = aws_eks_cluster.this.name
  addon_name                  = "kube-proxy"
  resolve_conflicts_on_create = "OVERWRITE"
  resolve_conflicts_on_update = "OVERWRITE"
  tags                        = local.common_tags
}

# -----------------------------------------------------------------------------
# Karpenter — IRSA Role (pre-provisioned for Helm install)
# -----------------------------------------------------------------------------
resource "aws_iam_role" "karpenter_controller" {
  name = "${local.cluster_name}-karpenter-controller"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action = "sts:AssumeRoleWithWebIdentity"
      Effect = "Allow"
      Principal = {
        Federated = aws_iam_openid_connect_provider.eks.arn
      }
      Condition = {
        StringEquals = {
          "${replace(aws_eks_cluster.this.identity[0].oidc[0].issuer, "https://", "")}:sub" = "system:serviceaccount:karpenter:karpenter"
          "${replace(aws_eks_cluster.this.identity[0].oidc[0].issuer, "https://", "")}:aud" = "sts.amazonaws.com"
        }
      }
    }]
  })

  tags = local.common_tags
}

resource "aws_iam_role_policy" "karpenter_controller" {
  name = "${local.cluster_name}-karpenter-policy"
  role = aws_iam_role.karpenter_controller.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "Karpenter"
        Effect = "Allow"
        Action = [
          "ec2:CreateFleet",
          "ec2:CreateLaunchTemplate",
          "ec2:CreateTags",
          "ec2:DeleteLaunchTemplate",
          "ec2:DescribeAvailabilityZones",
          "ec2:DescribeImages",
          "ec2:DescribeInstances",
          "ec2:DescribeInstanceTypeOfferings",
          "ec2:DescribeInstanceTypes",
          "ec2:DescribeLaunchTemplates",
          "ec2:DescribeSecurityGroups",
          "ec2:DescribeSubnets",
          "ec2:RunInstances",
          "ec2:TerminateInstances",
        ]
        Resource = "*"
      },
      {
        Sid    = "PassNodeRole"
        Effect = "Allow"
        Action = "iam:PassRole"
        Resource = aws_iam_role.node.arn
      },
      {
        Sid    = "EKSAccess"
        Effect = "Allow"
        Action = [
          "eks:DescribeCluster",
        ]
        Resource = aws_eks_cluster.this.arn
      },
      {
        Sid    = "SSMAccess"
        Effect = "Allow"
        Action = "ssm:GetParameter"
        Resource = "arn:${data.aws_partition.current.partition}:ssm:${data.aws_region.current.name}:*:parameter/aws/service/*"
      },
      {
        Sid    = "PricingAccess"
        Effect = "Allow"
        Action = [
          "pricing:GetProducts",
        ]
        Resource = "*"
      },
      {
        Sid    = "SQSAccess"
        Effect = "Allow"
        Action = [
          "sqs:DeleteMessage",
          "sqs:GetQueueUrl",
          "sqs:ReceiveMessage",
        ]
        Resource = aws_sqs_queue.karpenter_interruption.arn
      },
    ]
  })
}

# -----------------------------------------------------------------------------
# Karpenter — SQS Queue for spot interruption handling
# -----------------------------------------------------------------------------
resource "aws_sqs_queue" "karpenter_interruption" {
  name                      = "${local.cluster_name}-karpenter-interruption"
  message_retention_seconds = 300
  sqs_managed_sse_enabled   = true
  tags                      = local.common_tags
}

resource "aws_sqs_queue_policy" "karpenter_interruption" {
  queue_url = aws_sqs_queue.karpenter_interruption.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "EC2InterruptionPolicy"
        Effect = "Allow"
        Principal = {
          Service = ["events.amazonaws.com", "sqs.amazonaws.com"]
        }
        Action   = "sqs:SendMessage"
        Resource = aws_sqs_queue.karpenter_interruption.arn
      },
    ]
  })
}

# EventBridge rules for spot interruption events → SQS
resource "aws_cloudwatch_event_rule" "karpenter_interruption" {
  name        = "${local.cluster_name}-karpenter-interruption"
  description = "Forward EC2 interruption events to Karpenter SQS queue"

  event_pattern = jsonencode({
    source      = ["aws.ec2"]
    detail-type = ["EC2 Spot Instance Interruption Warning", "EC2 Instance State-change Notification"]
  })

  tags = local.common_tags
}

resource "aws_cloudwatch_event_target" "karpenter_interruption" {
  rule      = aws_cloudwatch_event_rule.karpenter_interruption.name
  target_id = "KarpenterInterruptionQueueTarget"
  arn       = aws_sqs_queue.karpenter_interruption.arn
}

# Instance profile for Karpenter-provisioned nodes
resource "aws_iam_instance_profile" "karpenter_node" {
  name = "${local.cluster_name}-karpenter-node"
  role = aws_iam_role.node.name
  tags = local.common_tags
}
