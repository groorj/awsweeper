package test

import (
	"fmt"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/cloudetc/awsweeper/command"
	res "github.com/cloudetc/awsweeper/resource"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
	"github.com/spf13/afero"
)

func TestAccElb_deleteByTags(t *testing.T) {
	t.SkipNow()
	// TODO tag support

	var lb1, lb2 elb.LoadBalancerDescription

	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:             testAccElbConfig,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAWSELBExists("aws_elb.foo", &lb1),
					testAccCheckAWSELBExists("aws_elb.bar", &lb2),
					testMainTags(argsDryRun, testAccELBAWSweeperTagsConfig),
					testElbExists(&lb1),
					testElbExists(&lb2),
					testMainTags(argsForceDelete, testAccELBAWSweeperTagsConfig),
					testElbDeleted(&lb1),
					testElbExists(&lb2),
				),
			},
		},
	})
}

func TestAccElb_deleteByIds(t *testing.T) {
	var lb1, lb2 elb.LoadBalancerDescription

	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:             testAccElbConfig,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAWSELBExists("aws_elb.foo", &lb1),
					testAccCheckAWSELBExists("aws_elb.bar", &lb2),
					testMainElbIds(argsDryRun, &lb1),
					testElbExists(&lb1),
					testElbExists(&lb2),
					testMainElbIds(argsForceDelete, &lb1),
					testElbDeleted(&lb1),
					testElbExists(&lb2),
				),
			},
		},
	})
}

func testAccCheckAWSELBExists(n string, res *elb.LoadBalancerDescription) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ELB ID is set")
		}

		conn := client.ELBconn

		describe, err := conn.DescribeLoadBalancers(&elb.DescribeLoadBalancersInput{
			LoadBalancerNames: []*string{aws.String(rs.Primary.ID)},
		})

		if err != nil {
			return err
		}

		if len(describe.LoadBalancerDescriptions) != 1 ||
			*describe.LoadBalancerDescriptions[0].LoadBalancerName != rs.Primary.ID {
			return fmt.Errorf("ELB not found")
		}

		*res = *describe.LoadBalancerDescriptions[0]

		// Confirm source_security_group_id for ELBs in a VPC
		// 	See https://github.com/hashicorp/terraform/pull/3780
		if res.VPCId != nil {
			sgid := rs.Primary.Attributes["source_security_group_id"]
			if sgid == "" {
				return fmt.Errorf("Expected to find source_security_group_id for ELB, but was empty")
			}
		}

		return nil
	}
}

func testMainElbIds(args []string, lb *elb.LoadBalancerDescription) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		res.AppFs = afero.NewMemMapFs()
		afero.WriteFile(res.AppFs, "config.yml", []byte(testAccElbAWSweeperIdsConfig(lb.LoadBalancerName)), 0644)
		os.Args = args

		command.WrappedMain()
		return nil
	}
}

func testElbExists(lb *elb.LoadBalancerDescription) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		conn := client.ELBconn

		DescribeElbOpts := &elb.DescribeLoadBalancersInput{
			LoadBalancerNames: []*string{lb.LoadBalancerName},
		}
		resp, err := conn.DescribeLoadBalancers(DescribeElbOpts)
		if err != nil {
			return err
		}

		if len(resp.LoadBalancerDescriptions) == 0 {
			return fmt.Errorf("ELB has been deleted")
		}

		return nil
	}
}

func testElbDeleted(lb *elb.LoadBalancerDescription) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		conn := client.ELBconn
		DescribeElbOpts := &elb.DescribeLoadBalancersInput{
			LoadBalancerNames: []*string{lb.LoadBalancerName},
		}
		resp, err := conn.DescribeLoadBalancers(DescribeElbOpts)
		if err != nil {
			elbErr, ok := err.(awserr.Error)
			if !ok {
				return err
			}
			if elbErr.Code() == "LoadBalancerNotFound" {
				return nil
			}
			return err
		}

		if len(resp.LoadBalancerDescriptions) != 0 {
			fmt.Println(resp.LoadBalancerDescriptions)
			return fmt.Errorf("ELB hasn't been deleted")

		}

		return nil
	}
}

const testAccElbConfig = `
resource "aws_elb" "foo" {
	name = "foo"
	subnets = [ "${aws_subnet.foo.id}" ]

	listener {
		instance_port = 80
		instance_protocol = "tcp"
		lb_port = 80
		lb_protocol = "tcp"
	}

	tags {
		foo = "bar"
	}
}

resource "aws_elb" "bar" {
	name = "bar"
	subnets = [ "${aws_subnet.foo.id}" ]

	listener {
		instance_port = 80
		instance_protocol = "tcp"
		lb_port = 80
		lb_protocol = "tcp"
	}

	tags {
		foo = "baz"
	}
}

resource "aws_vpc" "foo" {
	cidr_block = "10.1.0.0/16"

	tags {
		Name = "awsweeper-testacc"
	}
}

resource "aws_subnet" "foo" {
	vpc_id = "${aws_vpc.foo.id}"
	cidr_block = "10.1.0.1/24"

	tags {
		Name = "awsweeper-testacc"
	}
}

resource "aws_internet_gateway" "foo" {
  vpc_id = "${aws_vpc.foo.id}"

  tags {
	Name = "awsweeper-testacc"
  }
}
`

const testAccELBAWSweeperTagsConfig = `
aws_elb:
  tags:
    foo: bar
`

func testAccElbAWSweeperIdsConfig(id *string) string {
	return fmt.Sprintf(`
aws_elb:
  ids:
    - %s
`, *id)
}
