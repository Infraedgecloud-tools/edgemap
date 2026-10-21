provider "aws" {
  region = "eu-west-2"
}
resource "aws_vpc" "valid" {
  cidr_block = "10.70.0.0/16"
}
