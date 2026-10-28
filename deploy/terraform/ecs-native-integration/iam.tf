resource "aws_iam_role" "execution" {
  name = "${var.name}-execution"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{
    Effect = "Allow", Principal = { Service = "ecs-tasks.amazonaws.com" }, Action = "sts:AssumeRole"
  }] })
}
resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}
resource "aws_iam_role" "routing" {
  name = "${var.name}-routing"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{
    Effect = "Allow", Principal = { Service = "ecs.amazonaws.com" }, Action = "sts:AssumeRole"
  }] })
}
resource "aws_iam_role_policy_attachment" "routing" {
  role       = aws_iam_role.routing.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonECSInfrastructureRolePolicyForLoadBalancers"
}
resource "aws_iam_role" "tenet" {
  for_each = toset(["observer", "deployer"])
  name     = "${var.name}-${each.key}"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{
    Effect = "Allow", Principal = { AWS = var.runner_role_arn }, Action = "sts:AssumeRole"
  }] })
}
resource "aws_iam_role_policy" "read" {
  for_each = aws_iam_role.tenet
  role     = each.value.id
  policy = jsonencode({ Version = "2012-10-17", Statement = [
    {
      Effect = "Allow"
      Action = ["ecs:DescribeClusters", "ecs:ListServices", "ecs:DescribeServices", "ecs:ListServiceDeployments", "ecs:DescribeServiceDeployments", "ecs:DescribeServiceRevisions", "ecs:DescribeTasks"]
      Resource = concat([aws_ecs_cluster.test.arn, "${local.ecs_arn}:task/${var.name}/*"], flatten([
        for strategy in keys(local.strategies) : [
          "${local.ecs_arn}:service/${var.name}/${var.name}-${strategy}",
          "${local.ecs_arn}:service-deployment/${var.name}/${var.name}-${strategy}/*",
          "${local.ecs_arn}:service-revision/${var.name}/${var.name}-${strategy}/*",
          "${local.ecs_arn}:task-definition/${var.name}-${strategy}:*"
        ]
      ]))
    },
    {
      Effect   = "Allow"
      Action   = ["ecs:DescribeTaskDefinition", "ecs:ListTasks", "ecs:ListTaskDefinitions", "elasticloadbalancing:DescribeLoadBalancers", "elasticloadbalancing:DescribeListeners", "elasticloadbalancing:DescribeRules", "elasticloadbalancing:DescribeTargetGroups", "elasticloadbalancing:DescribeTargetHealth", "cloudwatch:DescribeAlarms", "cloudwatch:GetMetricData", "ec2:DescribeNetworkInterfaces"]
      Resource = "*"
    },
    {
      Effect   = "Allow", Action = ["iam:GetRole", "iam:SimulatePrincipalPolicy"]
      Resource = [for role in aws_iam_role.tenet : role.arn]
    }
  ] })
}
resource "aws_iam_role_policy" "deploy" {
  role = aws_iam_role.tenet["deployer"].id
  policy = jsonencode({ Version = "2012-10-17", Statement = [
    {
      Effect = "Allow", Action = ["ecs:UpdateService", "ecs:StopServiceDeployment"]
      Resource = flatten([for strategy in keys(local.strategies) : [
        "${local.ecs_arn}:service/${var.name}/${var.name}-${strategy}",
        "${local.ecs_arn}:service-deployment/${var.name}/${var.name}-${strategy}/*"
      ]])
    },
    # RegisterTaskDefinition does not support resource-level permissions.
    { Effect = "Allow", Action = "ecs:RegisterTaskDefinition", Resource = "*" },
    {
      Effect   = "Allow", Action = "ecs:TagResource"
      Resource = [for strategy in keys(local.strategies) : "${local.ecs_arn}:task-definition/${var.name}-${strategy}:*"]
    },
    {
      Effect    = "Allow", Action = "iam:PassRole", Resource = aws_iam_role.execution.arn
      Condition = { StringEquals = { "iam:PassedToService" = "ecs-tasks.amazonaws.com" } }
    },
    {
      Effect    = "Allow", Action = "iam:PassRole", Resource = aws_iam_role.routing.arn
      Condition = { StringEquals = { "iam:PassedToService" = "ecs.amazonaws.com" } }
    }
  ] })
}
