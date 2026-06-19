###############################################################################
# Amazon MSK (Managed Streaming for Kafka) — with per-tenant KMS isolation
#
# At-rest broker storage is encrypted with a platform CMK and in-transit
# traffic is TLS-only. Each tenant additionally receives a dedicated CMK used
# to envelope-encrypt tenant-scoped topic payloads at the application layer,
# keeping each venue's event stream cryptographically isolated. Tenant ID is
# never optional on the platform.
###############################################################################

locals {
  cluster_name = "${var.project_name}-${var.environment}"
  common_tags  = merge(var.tags, { Module = "msk", TaskID = "INFRA-1" })
}

# -----------------------------------------------------------------------------
# Platform CMK — MSK at-rest encryption
# -----------------------------------------------------------------------------
resource "aws_kms_key" "msk" {
  description             = "MSK at-rest encryption key for ${local.cluster_name}"
  deletion_window_in_days = 7
  enable_key_rotation     = true
  tags                    = merge(local.common_tags, { Name = "${local.cluster_name}-msk" })
}

resource "aws_kms_alias" "msk" {
  name          = "alias/${local.cluster_name}-msk"
  target_key_id = aws_kms_key.msk.key_id
}

# -----------------------------------------------------------------------------
# Per-tenant CMKs — tenant-scoped topic data isolation
# -----------------------------------------------------------------------------
resource "aws_kms_key" "tenant" {
  for_each = toset(var.tenant_ids)

  description             = "Tenant-scoped MSK CMK for ${each.key} (${local.cluster_name})"
  deletion_window_in_days = 7
  enable_key_rotation     = true

  tags = merge(local.common_tags, {
    Name     = "${local.cluster_name}-msk-${each.key}"
    TenantID = each.key
  })
}

resource "aws_kms_alias" "tenant" {
  for_each = toset(var.tenant_ids)

  name          = "alias/${local.cluster_name}-msk-${each.key}"
  target_key_id = aws_kms_key.tenant[each.key].key_id
}

# -----------------------------------------------------------------------------
# Networking — broker access from EKS nodes only
# -----------------------------------------------------------------------------
resource "aws_security_group" "msk" {
  name_prefix = "${local.cluster_name}-msk-"
  description = "MSK broker access from EKS nodes"
  vpc_id      = var.vpc_id

  ingress {
    description     = "Kafka TLS"
    from_port       = 9094
    to_port         = 9094
    protocol        = "tcp"
    security_groups = [var.eks_node_sg_id]
  }

  ingress {
    description     = "Kafka plaintext (in-VPC)"
    from_port       = 9092
    to_port         = 9092
    protocol        = "tcp"
    security_groups = [var.eks_node_sg_id]
  }

  ingress {
    description     = "Zookeeper"
    from_port       = 2181
    to_port         = 2181
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

  tags = merge(local.common_tags, { Name = "${local.cluster_name}-msk-sg" })

  lifecycle {
    create_before_destroy = true
  }
}

# -----------------------------------------------------------------------------
# Broker logging
# -----------------------------------------------------------------------------
resource "aws_cloudwatch_log_group" "msk" {
  name              = "/aws/msk/${local.cluster_name}"
  retention_in_days = 30
  tags              = local.common_tags
}

# -----------------------------------------------------------------------------
# Cluster configuration — topics are created explicitly per tenant, never auto
# -----------------------------------------------------------------------------
resource "aws_msk_configuration" "this" {
  name           = "${local.cluster_name}-config"
  kafka_versions = [var.kafka_version]

  server_properties = <<-PROPERTIES
    auto.create.topics.enable=false
    default.replication.factor=3
    min.insync.replicas=2
    num.partitions=6
    log.retention.hours=168
  PROPERTIES
}

# -----------------------------------------------------------------------------
# MSK cluster
# -----------------------------------------------------------------------------
resource "aws_msk_cluster" "this" {
  cluster_name           = local.cluster_name
  kafka_version          = var.kafka_version
  number_of_broker_nodes = var.broker_count

  broker_node_group_info {
    instance_type   = var.instance_type
    client_subnets  = var.data_subnet_ids
    security_groups = [aws_security_group.msk.id]

    storage_info {
      ebs_storage_info {
        volume_size = var.broker_volume_size
      }
    }
  }

  configuration_info {
    arn      = aws_msk_configuration.this.arn
    revision = aws_msk_configuration.this.latest_revision
  }

  encryption_info {
    encryption_at_rest_kms_key_arn = aws_kms_key.msk.arn

    encryption_in_transit {
      client_broker = "TLS"
      in_cluster    = true
    }
  }

  logging_info {
    broker_logs {
      cloudwatch_logs {
        enabled   = true
        log_group = aws_cloudwatch_log_group.msk.name
      }
    }
  }

  tags = local.common_tags
}
