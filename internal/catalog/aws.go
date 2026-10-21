// Package catalog describes AWS display and containment semantics.
package catalog

import "strings"

// Resource describes an AWS resource independently of dependency edges.
type Resource struct {
	Service          string
	Global           bool
	ParentAttributes []string
	Icon             string
}

var AWS = map[string]Resource{
	"aws_vpc":                             {Service: "VPC"},
	"aws_subnet":                          {Service: "Subnet", ParentAttributes: []string{"vpc_id"}},
	"aws_instance":                        {Service: "EC2", ParentAttributes: []string{"subnet_id"}, Icon: "Compute/Amazon-EC2.svg"},
	"aws_lb":                              {Service: "Load balancer", ParentAttributes: []string{"subnets", "subnet_id"}, Icon: "Networking%20&%20Content%20Delivery/Elastic-Load-Balancing.svg"},
	"aws_alb":                             {Service: "Load balancer", ParentAttributes: []string{"subnets", "subnet_id"}},
	"aws_autoscaling_group":               {Service: "Auto Scaling", ParentAttributes: []string{"vpc_zone_identifier"}},
	"aws_db_subnet_group":                 {Service: "DB subnet group", ParentAttributes: []string{"subnet_ids"}},
	"aws_db_instance":                     {Service: "RDS", ParentAttributes: []string{"db_subnet_group_name"}, Icon: "Database/Amazon-RDS.svg"},
	"aws_rds_cluster":                     {Service: "Aurora", ParentAttributes: []string{"db_subnet_group_name"}},
	"aws_rds_cluster_instance":            {Service: "Aurora instance", ParentAttributes: []string{"cluster_identifier", "db_subnet_group_name"}},
	"aws_lambda_function":                 {Service: "Lambda", ParentAttributes: []string{"subnet_ids"}, Icon: "Compute/AWS-Lambda.svg"},
	"aws_security_group":                  {Service: "Security group", ParentAttributes: []string{"vpc_id"}},
	"aws_security_group_rule":             {Service: "Security rule", ParentAttributes: []string{"security_group_id"}},
	"aws_vpc_security_group_ingress_rule": {Service: "Ingress rule", ParentAttributes: []string{"security_group_id"}},
	"aws_vpc_security_group_egress_rule":  {Service: "Egress rule", ParentAttributes: []string{"security_group_id"}},
	"aws_internet_gateway":                {Service: "Internet gateway", ParentAttributes: []string{"vpc_id"}},
	"aws_nat_gateway":                     {Service: "NAT gateway", ParentAttributes: []string{"subnet_id"}},
	"aws_route_table":                     {Service: "Route table", ParentAttributes: []string{"vpc_id"}},
	"aws_route_table_association":         {Service: "Route association", ParentAttributes: []string{"subnet_id"}},
	"aws_route":                           {Service: "Route", ParentAttributes: []string{"route_table_id"}},
	"aws_vpc_endpoint":                    {Service: "VPC endpoint", ParentAttributes: []string{"vpc_id"}},
	"aws_elasticache_subnet_group":        {Service: "Cache subnet group", ParentAttributes: []string{"subnet_ids"}},
	"aws_elasticache_cluster":             {Service: "ElastiCache", ParentAttributes: []string{"subnet_group_name"}},
	"aws_efs_mount_target":                {Service: "EFS mount", ParentAttributes: []string{"subnet_id"}},
	"aws_s3_bucket":                       {Service: "S3", Global: true, Icon: "Storage/Amazon-Simple-Storage-Service-S3.svg"},
}

func Lookup(kind string) (Resource, bool) {
	if resource, ok := AWS[kind]; ok {
		return resource, true
	}
	if strings.HasPrefix(kind, "aws_iam_") || strings.HasPrefix(kind, "aws_route53_") || strings.HasPrefix(kind, "aws_cloudfront_") || strings.HasPrefix(kind, "aws_s3_") {
		return Resource{Service: kind, Global: true}, true
	}
	return Resource{}, false
}
