locals {
  deployments = { for key, strategy in local.strategies : key => {
    name     = "${var.name}-${key}", appRef = "${var.name}-${key}", environmentRef = "integration"
    target   = { type = "ecs" }
    strategy = { type = key == "blue-green" ? "blueGreen" : key }
    image    = var.image, capacity = { desired = 2, min = 1, max = 4 }
    ecs = {
      deploymentController = "ECS", clusterArn = aws_ecs_cluster.test.arn
      serviceArn           = aws_ecs_service.test[key].id, baseTaskDefinitionArn = aws_ecs_task_definition.baseline[key].arn
      containerName        = "web", syncPolicy = { mode = "manual", selfHeal = false }, rollbackOnFailure = false
      advanced = merge({
        bakeTimeInMinutes = 2
        routing = {
          targetGroupArn            = aws_lb_target_group.blue[key].arn, alternateTargetGroupArn = aws_lb_target_group.green[key].arn
          productionListenerRuleArn = aws_lb_listener_rule.production[key].arn
          infrastructureRoleArn     = aws_iam_role.routing.arn, containerPort = 8080
        }
        alarms = { enable = true, rollback = false, names = [aws_cloudwatch_metric_alarm.failure[key].alarm_name] }
        }, key == "canary" ? { canary = { percent = 25, bakeTimeInMinutes = 2 } } : {},
      key == "linear" ? { linear = { stepPercent = 25, stepBakeTimeInMinutes = 2 } } : {})
    }
  } }
}
output "repository_files" {
  description = "JSON documents (valid YAML) for a separate test Git repository; all execution policies default off."
  value = merge({
    "accounts/integration.yaml" = jsonencode({
      name    = "integration", awsAccountId = var.account_id, allowedRegions = [var.region]
      roleArn = aws_iam_role.tenet["deployer"].arn, observerRoleArn = aws_iam_role.tenet["observer"].arn
    })
    "environments/integration.yaml" = jsonencode({ name = "integration", accountRef = "integration", region = var.region })
  }, { for key, deployment in local.deployments : "deployments/${key}.yaml" => jsonencode(deployment) })
}
output "test_bindings" {
  value = {
    account_id   = var.account_id, region = var.region, cluster_arn = aws_ecs_cluster.test.arn
    endpoint     = "http://${aws_lb.test.dns_name}", runner_role_arn = var.runner_role_arn
    assume_roles = [for role in aws_iam_role.tenet : role.arn]
    services = { for key, strategy in local.strategies : key => {
      service_arn       = aws_ecs_service.test[key].id, alarm_name = aws_cloudwatch_metric_alarm.failure[key].alarm_name
      rule_arn          = aws_lb_listener_rule.production[key].arn
      target_group_arns = [aws_lb_target_group.blue[key].arn, aws_lb_target_group.green[key].arn]
    } }
  }
}
