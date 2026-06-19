###############################################################################
# Per-Tenant IRSA Roles + KMS CMKs
#
# Each tenant (venue) receives:
#   - a dedicated IAM Role for Service Accounts (IRSA), assumable ONLY by
#     Kubernetes service accounts in that tenant's namespace, and
#   - a dedicated customer-managed KMS key (CMK) for tenant-isolated data.
#
# This enforces the GarudaX platform invariant that tenant resources are
# both identity- and cryptographically-isolated. Tenant ID is never optional.
#
# Re-uses data sources declared in main.tf:
#   data.aws_caller_identity.current, data.aws_partition.current
###############################################################################

locals {
  # Kubernetes namespace convention for tenant workloads: tenant-<tenant_id>
  tenant_namespaces = { for t in var.tenant_ids : t => "tenant-${t}" }

  # OIDC issuer host (without scheme), used to scope IRSA trust conditions.
  oidc_issuer = replace(aws_eks_cluster.this.identity[0].oidc[0].issuer, "https://", "")
}

# -----------------------------------------------------------------------------
# Per-tenant IRSA role — assumable only by service accounts in the tenant ns
# -----------------------------------------------------------------------------
resource "aws_iam_role" "tenant_irsa" {
  for_each = toset(var.tenant_ids)

  name = "${local.cluster_name}-${each.key}-irsa"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action = "sts:AssumeRoleWithWebIdentity"
      Effect = "Allow"
      Principal = {
        Federated = aws_iam_openid_connect_provider.eks.arn
      }
      Condition = {
        # StringLike allows the trailing wildcard so any service account in the
        # tenant namespace can assume the role, but no other namespace can.
        StringLike = {
          "${local.oidc_issuer}:sub" = "system:serviceaccount:${local.tenant_namespaces[each.key]}:*"
        }
        StringEquals = {
          "${local.oidc_issuer}:aud" = "sts.amazonaws.com"
        }
      }
    }]
  })

  tags = merge(local.common_tags, {
    Name     = "${local.cluster_name}-${each.key}-irsa"
    TenantID = each.key
  })
}

# -----------------------------------------------------------------------------
# Per-tenant KMS CMK — envelope encryption for tenant-isolated data
# -----------------------------------------------------------------------------
resource "aws_kms_key" "tenant" {
  for_each = toset(var.tenant_ids)

  description             = "Tenant-scoped CMK for ${each.key} on ${local.cluster_name}"
  deletion_window_in_days = 7
  enable_key_rotation     = true

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "EnableRootAccountAdmin"
        Effect = "Allow"
        Principal = {
          AWS = "arn:${data.aws_partition.current.partition}:iam::${data.aws_caller_identity.current.account_id}:root"
        }
        Action   = "kms:*"
        Resource = "*"
      },
      {
        Sid    = "AllowTenantIRSAUse"
        Effect = "Allow"
        Principal = {
          AWS = aws_iam_role.tenant_irsa[each.key].arn
        }
        Action = [
          "kms:Encrypt",
          "kms:Decrypt",
          "kms:ReEncrypt*",
          "kms:GenerateDataKey*",
          "kms:DescribeKey",
        ]
        Resource = "*"
      },
    ]
  })

  tags = merge(local.common_tags, {
    Name     = "${local.cluster_name}-${each.key}-tenant"
    TenantID = each.key
  })
}

resource "aws_kms_alias" "tenant" {
  for_each = toset(var.tenant_ids)

  name          = "alias/${local.cluster_name}-${each.key}-tenant"
  target_key_id = aws_kms_key.tenant[each.key].key_id
}

# -----------------------------------------------------------------------------
# Per-tenant IAM policy — restricts each tenant role to ITS OWN CMK only
# -----------------------------------------------------------------------------
resource "aws_iam_role_policy" "tenant_irsa_kms" {
  for_each = toset(var.tenant_ids)

  name = "${local.cluster_name}-${each.key}-kms"
  role = aws_iam_role.tenant_irsa[each.key].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid    = "TenantScopedKMS"
      Effect = "Allow"
      Action = [
        "kms:Encrypt",
        "kms:Decrypt",
        "kms:ReEncrypt*",
        "kms:GenerateDataKey*",
        "kms:DescribeKey",
      ]
      Resource = aws_kms_key.tenant[each.key].arn
    }]
  })
}
