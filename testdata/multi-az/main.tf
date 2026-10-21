provider "aws" {
  region = "eu-west-2"
}
provider "aws" {
  alias  = "dr"
  region = "eu-west-1"
}
resource "aws_vpc" "app" {
  cidr_block = "10.20.0.0/16"
}
resource "aws_subnet" "public_a" {
  vpc_id                  = aws_vpc.app.id
  cidr_block              = "10.20.1.0/24"
  availability_zone       = "eu-west-2a"
  map_public_ip_on_launch = true
}
resource "aws_subnet" "public_b" {
  vpc_id                  = aws_vpc.app.id
  cidr_block              = "10.20.2.0/24"
  availability_zone       = "eu-west-2b"
  map_public_ip_on_launch = false
}
resource "aws_internet_gateway" "app" {
  vpc_id = aws_vpc.app.id
}
resource "aws_route_table" "public" {
  vpc_id = aws_vpc.app.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.app.id
  }
}
resource "aws_route_table_association" "public_b" {
  subnet_id      = aws_subnet.public_b.id
  route_table_id = aws_route_table.public.id
}
resource "aws_subnet" "private_a" {
  vpc_id            = aws_vpc.app.id
  cidr_block        = "10.20.11.0/24"
  availability_zone = "eu-west-2a"
  tags              = { Name = "private-a" }
}
resource "aws_subnet" "private_b" {
  vpc_id            = aws_vpc.app.id
  cidr_block        = "10.20.12.0/24"
  availability_zone = "eu-west-2b"
  tags              = { Name = "private-b" }
}
resource "aws_lb" "web" {
  name               = "web"
  load_balancer_type = "application"
  subnets            = [aws_subnet.public_a.id, aws_subnet.public_b.id]
}
resource "aws_db_subnet_group" "data" {
  subnet_ids = [aws_subnet.private_a.id, aws_subnet.private_b.id]
}
resource "aws_db_instance" "data" {
  engine               = "postgres"
  instance_class       = "db.t3.micro"
  allocated_storage    = 20
  db_subnet_group_name = aws_db_subnet_group.data.name
}
resource "aws_autoscaling_group" "mixed" {
  min_size            = 0
  max_size            = 2
  vpc_zone_identifier = [aws_subnet.public_a.id, aws_subnet.private_a.id]
}
resource "aws_vpc" "dr" {
  provider   = aws.dr
  cidr_block = "10.30.0.0/16"
}
