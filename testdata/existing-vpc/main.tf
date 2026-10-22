provider "aws" {
  region = "eu-west-1"
}
# The shared network is owned elsewhere; this configuration only adds to it.
data "aws_vpc" "shared" {
  tags = { Name = "shared" }
}
data "aws_internet_gateway" "shared" {
  filter {
    name   = "attachment.vpc-id"
    values = [data.aws_vpc.shared.id]
  }
}
resource "aws_subnet" "web" {
  vpc_id                  = data.aws_vpc.shared.id
  cidr_block              = "10.20.1.0/24"
  map_public_ip_on_launch = true
}
resource "aws_subnet" "app" {
  vpc_id     = data.aws_vpc.shared.id
  cidr_block = "10.20.11.0/24"
  tags       = { Name = "app-private" }
}
resource "aws_lb" "web" {
  name               = "web"
  load_balancer_type = "application"
  subnets            = [aws_subnet.web.id]
}
resource "aws_instance" "api" {
  ami           = "ami-example"
  instance_type = "t3.small"
  subnet_id     = aws_subnet.app.id
}
