resource "aws_vpc" "main" {
  cidr_block = "10.40.0.0/16"
}
resource "aws_subnet" "public" {
  vpc_id                  = aws_vpc.main.id
  cidr_block              = "10.40.1.0/24"
  map_public_ip_on_launch = true
}
output "public_subnet_id" {
  value = aws_subnet.public.id
}
output "vpc_id" {
  value = aws_vpc.main.id
}
