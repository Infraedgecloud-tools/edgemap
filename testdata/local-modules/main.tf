provider "aws" {
  region = "eu-west-2"
}
module "network" {
  source = "./modules/network"
}
resource "aws_instance" "web" {
  ami           = "ami-example"
  instance_type = "t3.micro"
  subnet_id     = module.network.public_subnet_id
  depends_on    = [module.network]
}
