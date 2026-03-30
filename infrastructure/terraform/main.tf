terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  backend "s3" {
    bucket         = "garudax-platform-tfstate"
    key            = "infrastructure/terraform.tfstate"
    region         = "eu-west-1"  # Out-of-band from primary region
    dynamodb_table = "garudax-platform-tflock"
    encrypt        = true
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project   = var.project_name
      ManagedBy = "terraform"
    }
  }
}

provider "aws" {
  alias  = "dr"
  region = var.dr_region

  default_tags {
    tags = {
      Project   = var.project_name
      ManagedBy = "terraform"
    }
  }
}

# --- Module composition ---
# Modules are wired together here. Each module is self-contained.

module "vpc" {
  source       = "./modules/vpc"
  project_name = var.project_name
  environment  = var.environment
  vpc_cidr     = var.vpc_cidr
  tags         = var.tags
}

module "eks" {
  source              = "./modules/eks"
  project_name        = var.project_name
  environment         = var.environment
  cluster_version     = var.eks_cluster_version
  vpc_id              = module.vpc.vpc_id
  private_subnet_ids  = module.vpc.private_app_subnet_ids
  node_groups         = var.eks_node_groups
  tags                = var.tags
}

module "rds" {
  source             = "./modules/rds"
  project_name       = var.project_name
  environment        = var.environment
  instance_class     = var.rds_instance_class
  allocated_storage  = var.rds_allocated_storage
  multi_az           = var.rds_multi_az
  vpc_id             = module.vpc.vpc_id
  data_subnet_ids    = module.vpc.private_data_subnet_ids
  eks_node_sg_id     = module.eks.node_security_group_id
  tags               = var.tags
}

module "msk" {
  source          = "./modules/msk"
  project_name    = var.project_name
  environment     = var.environment
  instance_type   = var.msk_instance_type
  broker_count    = var.msk_broker_count
  vpc_id          = module.vpc.vpc_id
  data_subnet_ids = module.vpc.private_data_subnet_ids
  eks_node_sg_id  = module.eks.node_security_group_id
  tags            = var.tags
}

module "security_groups" {
  source       = "./modules/security-groups"
  project_name = var.project_name
  environment  = var.environment
  vpc_id       = module.vpc.vpc_id
  tags         = var.tags
}
