provider "aws" {
  region = "eu-west-2"
}
resource "aws_vpc" "app" {
  cidr_block = "10.10.0.0/16"
  tags = { Name = "app" }
}
resource "aws_subnet" "public" {
  vpc_id                  = aws_vpc.app.id
  cidr_block              = "10.10.1.0/24"
  map_public_ip_on_launch = true
}
resource "aws_subnet" "private" {
  vpc_id     = aws_vpc.app.id
  cidr_block = "10.10.2.0/24"
  tags       = { Name = "private-data" }
}
resource "aws_security_group" "web" {
  name   = "web"
  vpc_id = aws_vpc.app.id
  dynamic "ingress" {
    for_each = [443]
    content {
      from_port   = ingress.value
      to_port     = ingress.value
      protocol    = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
    }
  }
}
resource "aws_iam_role" "web" {
  name               = "web"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [] })
}
resource "aws_iam_instance_profile" "web" {
  role = aws_iam_role.web.name
}
resource "aws_instance" "web" {
  ami                    = "ami-example"
  instance_type          = "t3.micro"
  subnet_id              = aws_subnet.public.id
  vpc_security_group_ids = [aws_security_group.web.id]
  iam_instance_profile   = aws_iam_instance_profile.web.name
  tags                   = { Name = "web" }
}
resource "aws_db_subnet_group" "data" {
  subnet_ids = [aws_subnet.private.id]
}
resource "aws_db_instance" "data" {
  engine               = "postgres"
  instance_class       = "db.t3.micro"
  db_subnet_group_name = aws_db_subnet_group.data.name
  allocated_storage    = 20
}
data "aws_caller_identity" "current" {}
resource "aws_s3_bucket" "logs" {
  bucket = "fixture-logs-${data.aws_caller_identity.current.account_id}"
}
