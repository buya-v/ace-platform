###############################################################################
# RDS PostgreSQL — platform database with per-tenant KMS isolation
#
# Storage and the AWS-managed master secret are encrypted with a platform CMK.
# Each tenant additionally receives a dedicated CMK so tenant-scoped data
# (logical backups, exports, per-tenant snapshots) can be encrypted under a key
# that only that tenant's workloads are granted access to. Tenant ID is never
# optional on the platform.
###############################################################################

data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}

locals {
  identifier  = "${var.project_name}-${var.environment}"
  common_tags = merge(var.tags, { Module = "rds", TaskID = "INFRA-1" })
}

# -----------------------------------------------------------------------------
# Platform CMK — RDS storage + master user secret encryption
# -----------------------------------------------------------------------------
resource "aws_kms_key" "rds" {
  description             = "RDS storage encryption key for ${local.identifier}"
  deletion_window_in_days = 7
  enable_key_rotation     = true
  tags                    = merge(local.common_tags, { Name = "${local.identifier}-rds" })
}

resource "aws_kms_alias" "rds" {
  name          = "alias/${local.identifier}-rds"
  target_key_id = aws_kms_key.rds.key_id
}

# -----------------------------------------------------------------------------
# Per-tenant CMKs — tenant-isolated data encryption (backups / exports)
# -----------------------------------------------------------------------------
resource "aws_kms_key" "tenant" {
  for_each = toset(var.tenant_ids)

  description             = "Tenant-scoped RDS data CMK for ${each.key} (${local.identifier})"
  deletion_window_in_days = 7
  enable_key_rotation     = true

  tags = merge(local.common_tags, {
    Name     = "${local.identifier}-rds-${each.key}"
    TenantID = each.key
  })
}

resource "aws_kms_alias" "tenant" {
  for_each = toset(var.tenant_ids)

  name          = "alias/${local.identifier}-rds-${each.key}"
  target_key_id = aws_kms_key.tenant[each.key].key_id
}

# -----------------------------------------------------------------------------
# Networking — subnet group + access from EKS nodes only
# -----------------------------------------------------------------------------
resource "aws_db_subnet_group" "this" {
  name       = "${local.identifier}-rds"
  subnet_ids = var.data_subnet_ids
  tags       = local.common_tags
}

resource "aws_security_group" "rds" {
  name_prefix = "${local.identifier}-rds-"
  description = "RDS PostgreSQL access from EKS nodes"
  vpc_id      = var.vpc_id

  ingress {
    description     = "PostgreSQL from EKS nodes"
    from_port       = 5432
    to_port         = 5432
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

  tags = merge(local.common_tags, { Name = "${local.identifier}-rds-sg" })

  lifecycle {
    create_before_destroy = true
  }
}

# -----------------------------------------------------------------------------
# RDS PostgreSQL instance
# -----------------------------------------------------------------------------
resource "aws_db_instance" "this" {
  identifier     = local.identifier
  engine         = "postgres"
  engine_version = var.engine_version
  instance_class = var.instance_class

  allocated_storage     = var.allocated_storage
  max_allocated_storage = var.allocated_storage * 2
  storage_type          = "gp3"
  storage_encrypted     = true
  kms_key_id            = aws_kms_key.rds.arn

  db_name  = var.database_name
  username = var.master_username
  port     = 5432

  # Master password is generated and rotated by AWS Secrets Manager, encrypted
  # with the platform CMK — no plaintext secret ever enters Terraform state.
  manage_master_user_password   = true
  master_user_secret_kms_key_id = aws_kms_key.rds.arn

  multi_az               = var.multi_az
  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  backup_retention_period   = var.backup_retention_period
  deletion_protection       = var.environment == "prod"
  skip_final_snapshot       = var.environment != "prod"
  final_snapshot_identifier = var.environment == "prod" ? "${local.identifier}-final" : null

  performance_insights_enabled    = true
  performance_insights_kms_key_id = aws_kms_key.rds.arn

  tags = local.common_tags
}
