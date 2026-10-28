# All provider calls are mocked: no credentials, AWS requests or resources.
mock_provider "aws" {}

variables {
  account_id         = "123456789012"
  availability_zones = ["ap-northeast-2a", "ap-northeast-2c"]
  operator_cidr      = "192.0.2.10/32"
  runner_role_arn    = "arn:aws:iam::123456789012:role/test-runner"
  image              = "123456789012.dkr.ecr.ap-northeast-2.amazonaws.com/test@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
}

run "fixture_contract" {
  command = plan
  assert {
    condition     = toset(keys(aws_ecs_service.test)) == toset(["blue-green", "canary", "linear"])
    error_message = "Each native strategy needs an isolated service."
  }
  assert {
    condition = alltrue([for key, service in aws_ecs_service.test :
      service.deployment_controller[0].type == "ECS" &&
      service.deployment_configuration[0].strategy == local.strategies[key] &&
      service.desired_count == 2 && service.alarms[0].rollback == false
    ])
    error_message = "Bootstrap must use native ECS, baseline count 2 and independently testable Tenet rollback."
  }
  assert {
    condition = alltrue([for deployment in local.deployments :
      deployment.ecs.syncPolicy.mode == "manual" && !deployment.ecs.syncPolicy.selfHeal &&
      !deployment.ecs.rollbackOnFailure && deployment.image == var.image
    ])
    error_message = "Generated Git inputs must pin the workload and require explicit automation enablement."
  }
  assert {
    condition = alltrue([for group in concat(values(aws_lb_target_group.blue), values(aws_lb_target_group.green)) :
      group.target_type == "ip" && group.health_check[0].path == "/health"
    ])
    error_message = "All services require two IP target groups with health checks."
  }
  assert {
    condition     = aws_vpc_security_group_ingress_rule.operator.cidr_ipv4 == var.operator_cidr && aws_vpc_security_group_ingress_rule.alb_to_tasks.cidr_ipv4 == null
    error_message = "ALB ingress must be operator-scoped; task ingress must be from the ALB group only."
  }
  assert {
    condition = alltrue([for task in aws_ecs_task_definition.baseline :
      jsondecode(task.container_definitions)[0].healthCheck.command[0] == "CMD" &&
      task.network_mode == "awsvpc"
    ])
    error_message = "Baseline must provide both container and ALB health evidence."
  }
}

run "reject_public_ingress" {
  command = plan
  variables { operator_cidr = "0.0.0.0/0" }
  expect_failures = [var.operator_cidr]
}
run "reject_mutable_image" {
  command = plan
  variables { image = "example.com/test:latest" }
  expect_failures = [var.image]
}
run "reject_single_zone" {
  command = plan
  variables { availability_zones = ["ap-northeast-2a", "ap-northeast-2a"] }
  expect_failures = [var.availability_zones]
}
