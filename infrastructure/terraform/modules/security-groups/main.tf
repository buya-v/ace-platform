###############################################################################
# Security Groups — shared platform-level groups
#
# Per-service data-plane groups (RDS, MSK, EKS nodes) live with their own
# modules where they can reference the consuming security group directly. This
# module owns the cross-cutting groups that are not owned by any single tier:
#   * alb   — public-facing application load balancers
#   * app   — east-west traffic between application workloads, fronted by the ALB
###############################################################################

locals {
  identifier  = "${var.project_name}-${var.environment}"
  common_tags = merge(var.tags, { Module = "security-groups", TaskID = "INFRA-1" })
}

# -----------------------------------------------------------------------------
# ALB — internet-facing ingress (HTTPS only)
# -----------------------------------------------------------------------------
resource "aws_security_group" "alb" {
  name_prefix = "${local.identifier}-alb-"
  description = "Public ALB ingress for the platform edge"
  vpc_id      = var.vpc_id

  ingress {
    description = "HTTPS from the internet"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    description = "Allow all outbound"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, { Name = "${local.identifier}-alb-sg" })

  lifecycle {
    create_before_destroy = true
  }
}

# -----------------------------------------------------------------------------
# App — workload traffic, reachable only from the ALB
# -----------------------------------------------------------------------------
resource "aws_security_group" "app" {
  name_prefix = "${local.identifier}-app-"
  description = "Application workload traffic fronted by the platform ALB"
  vpc_id      = var.vpc_id

  ingress {
    description     = "App traffic from the ALB"
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  egress {
    description = "Allow all outbound"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, { Name = "${local.identifier}-app-sg" })

  lifecycle {
    create_before_destroy = true
  }
}
