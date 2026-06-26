###############################################################################
# ElastiCache for Redis — clustered, in-transit + at-rest encrypted
#
# Cluster-mode replication group (sharded) reachable only from EKS worker nodes.
# At-rest storage is encrypted with a platform CMK and all client traffic is
# TLS-only. Tenant isolation for the cache layer is enforced logically (keyspace
# prefixing per tenant) rather than per-CMK, since a single replication group
# serves all venues; the platform invariant (tenant ID never optional) is
# enforced at the application layer that owns the keyspace.
###############################################################################

locals {
  cluster_name = "${var.project_name}-${var.environment}"
  common_tags  = merge(var.tags, { Module = "elasticache", TaskID = "INFRA-1" })
}

# -----------------------------------------------------------------------------
# Platform CMK — at-rest encryption
# -----------------------------------------------------------------------------
resource "aws_kms_key" "redis" {
  description             = "ElastiCache Redis at-rest encryption key for ${local.cluster_name}"
  deletion_window_in_days = 7
  enable_key_rotation     = true
  tags                    = merge(local.common_tags, { Name = "${local.cluster_name}-redis" })
}

resource "aws_kms_alias" "redis" {
  name          = "alias/${local.cluster_name}-redis"
  target_key_id = aws_kms_key.redis.key_id
}

# -----------------------------------------------------------------------------
# Networking — access from EKS nodes only
# -----------------------------------------------------------------------------
resource "aws_elasticache_subnet_group" "this" {
  name       = "${local.cluster_name}-redis"
  subnet_ids = var.data_subnet_ids
  tags       = local.common_tags
}

resource "aws_security_group" "redis" {
  name_prefix = "${local.cluster_name}-redis-"
  description = "ElastiCache Redis access from EKS nodes"
  vpc_id      = var.vpc_id

  ingress {
    description     = "Redis from EKS nodes"
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [var.eks_node_sg_id]
  }

  egress {
    description = "Allow all outbound"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, { Name = "${local.cluster_name}-redis-sg" })

  lifecycle {
    create_before_destroy = true
  }
}

# -----------------------------------------------------------------------------
# Replication group — cluster mode enabled (sharded)
# -----------------------------------------------------------------------------
resource "aws_elasticache_replication_group" "this" {
  replication_group_id = "${local.cluster_name}-redis"
  description          = "Platform Redis for ${local.cluster_name}"

  engine    = "redis"
  node_type = var.node_type
  port      = 6379

  num_node_groups         = var.num_shards
  replicas_per_node_group = var.replicas_per_shard

  subnet_group_name  = aws_elasticache_subnet_group.this.name
  security_group_ids = [aws_security_group.redis.id]

  at_rest_encryption_enabled = true
  kms_key_id                 = aws_kms_key.redis.arn
  transit_encryption_enabled = true

  automatic_failover_enabled = true
  multi_az_enabled           = var.environment == "prod"

  tags = local.common_tags
}
