variable "region" {
  type = string
}
variable "workers" {
  type = map(string)
}
provider "aws" {
  region = var.region
}
locals {
  network_cidr = "10.60.0.0/16"
}
resource "aws_vpc" "app" {
  cidr_block = local.network_cidr
}
resource "aws_subnet" "private" {
  vpc_id     = aws_vpc.app.id
  cidr_block = "10.60.1.0/24"
  tags       = { Name = "private-workers" }
}
resource "aws_instance" "workers" {
  for_each      = var.workers
  ami           = each.value
  instance_type = "t3.micro"
  subnet_id     = aws_subnet.private.id
}
resource "aws_instance" "fixed" {
  count         = 3
  ami           = "ami-example"
  instance_type = "t3.micro"
  subnet_id     = aws_subnet.private.id
}
