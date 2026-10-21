provider "aws" {
  region = "eu-west-2"
}
module "vpc" {
  source          = "terraform-aws-modules/vpc/aws"
  version         = "5.21.0"
  name            = "app"
  cidr            = "10.50.0.0/16"
  azs             = ["eu-west-2a", "eu-west-2b"]
  private_subnets = ["10.50.11.0/24", "10.50.12.0/24"]
  public_subnets  = ["10.50.1.0/24", "10.50.2.0/24"]
}
resource "aws_lb" "web" {
  name               = "web"
  load_balancer_type = "application"
  subnets            = module.vpc.public_subnets
}
resource "aws_db_subnet_group" "data" {
  subnet_ids = module.vpc.private_subnets
}
resource "aws_db_instance" "data" {
  engine               = "postgres"
  instance_class       = "db.t3.micro"
  allocated_storage    = 20
  db_subnet_group_name = aws_db_subnet_group.data.name
}
resource "aws_security_group" "app" {
  name   = "app"
  vpc_id = module.vpc.vpc_id
}
