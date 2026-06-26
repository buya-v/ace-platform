###############################################################################
# VPC — platform network foundation
#
# A three-tier network spread across the region's first three Availability
# Zones:
#   * public         — internet-facing (NAT gateways, future ALBs)
#   * private_app     — EKS worker nodes / application workloads
#   * private_data    — stateful services (RDS, MSK) with no inbound internet
#
# Each AZ gets its own NAT gateway so a single-AZ failure cannot sever egress
# for the surviving zones. Outputs feed the eks, rds, and msk modules.
###############################################################################

data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  identifier  = "${var.project_name}-${var.environment}"
  common_tags = merge(var.tags, { Module = "vpc", TaskID = "INFRA-1" })

  az_count = 3
  azs      = slice(data.aws_availability_zones.available.names, 0, local.az_count)

  # /16 VPC carved into /20 subnets (newbits = 4 -> 16 blocks).
  # Reserve distinct index ranges per tier so the layout is stable.
  public_cidrs       = [for i in range(local.az_count) : cidrsubnet(var.vpc_cidr, 4, i)]
  private_app_cidrs  = [for i in range(local.az_count) : cidrsubnet(var.vpc_cidr, 4, i + 3)]
  private_data_cidrs = [for i in range(local.az_count) : cidrsubnet(var.vpc_cidr, 4, i + 6)]
}

# -----------------------------------------------------------------------------
# VPC + Internet Gateway
# -----------------------------------------------------------------------------
resource "aws_vpc" "this" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = merge(local.common_tags, { Name = local.identifier })
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id
  tags   = merge(local.common_tags, { Name = "${local.identifier}-igw" })
}

# -----------------------------------------------------------------------------
# Subnets
# -----------------------------------------------------------------------------
resource "aws_subnet" "public" {
  count = local.az_count

  vpc_id                  = aws_vpc.this.id
  cidr_block              = local.public_cidrs[count.index]
  availability_zone       = local.azs[count.index]
  map_public_ip_on_launch = true

  tags = merge(local.common_tags, {
    Name                     = "${local.identifier}-public-${local.azs[count.index]}"
    Tier                     = "public"
    "kubernetes.io/role/elb" = "1"
  })
}

resource "aws_subnet" "private_app" {
  count = local.az_count

  vpc_id            = aws_vpc.this.id
  cidr_block        = local.private_app_cidrs[count.index]
  availability_zone = local.azs[count.index]

  tags = merge(local.common_tags, {
    Name                              = "${local.identifier}-private-app-${local.azs[count.index]}"
    Tier                              = "private-app"
    "kubernetes.io/role/internal-elb" = "1"
  })
}

resource "aws_subnet" "private_data" {
  count = local.az_count

  vpc_id            = aws_vpc.this.id
  cidr_block        = local.private_data_cidrs[count.index]
  availability_zone = local.azs[count.index]

  tags = merge(local.common_tags, {
    Name = "${local.identifier}-private-data-${local.azs[count.index]}"
    Tier = "private-data"
  })
}

# -----------------------------------------------------------------------------
# NAT gateways — one per AZ for resilient egress
# -----------------------------------------------------------------------------
resource "aws_eip" "nat" {
  count  = local.az_count
  domain = "vpc"
  tags   = merge(local.common_tags, { Name = "${local.identifier}-nat-${local.azs[count.index]}" })

  depends_on = [aws_internet_gateway.this]
}

resource "aws_nat_gateway" "this" {
  count = local.az_count

  allocation_id = aws_eip.nat[count.index].id
  subnet_id     = aws_subnet.public[count.index].id

  tags = merge(local.common_tags, { Name = "${local.identifier}-nat-${local.azs[count.index]}" })

  depends_on = [aws_internet_gateway.this]
}

# -----------------------------------------------------------------------------
# Routing
# -----------------------------------------------------------------------------
resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.this.id
  }

  tags = merge(local.common_tags, { Name = "${local.identifier}-public" })
}

resource "aws_route_table_association" "public" {
  count          = local.az_count
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

# Private route tables are per-AZ so each tier egresses through its own AZ's NAT.
resource "aws_route_table" "private" {
  count  = local.az_count
  vpc_id = aws_vpc.this.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.this[count.index].id
  }

  tags = merge(local.common_tags, { Name = "${local.identifier}-private-${local.azs[count.index]}" })
}

resource "aws_route_table_association" "private_app" {
  count          = local.az_count
  subnet_id      = aws_subnet.private_app[count.index].id
  route_table_id = aws_route_table.private[count.index].id
}

resource "aws_route_table_association" "private_data" {
  count          = local.az_count
  subnet_id      = aws_subnet.private_data[count.index].id
  route_table_id = aws_route_table.private[count.index].id
}
