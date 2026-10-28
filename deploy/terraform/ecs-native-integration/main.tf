locals {
  strategies = {
    blue-green = "BLUE_GREEN"
    canary     = "CANARY"
    linear     = "LINEAR"
  }
  ecs_arn = "arn:aws:ecs:${var.region}:${var.account_id}"
}

resource "aws_vpc" "test" {
  cidr_block           = "10.83.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true
}
resource "aws_internet_gateway" "test" {
  vpc_id = aws_vpc.test.id
}
resource "aws_subnet" "test" {
  count             = 2
  vpc_id            = aws_vpc.test.id
  cidr_block        = cidrsubnet(aws_vpc.test.cidr_block, 8, count.index)
  availability_zone = var.availability_zones[count.index]
}
resource "aws_route_table" "test" {
  vpc_id = aws_vpc.test.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.test.id
  }
}
resource "aws_route_table_association" "test" {
  count          = 2
  subnet_id      = aws_subnet.test[count.index].id
  route_table_id = aws_route_table.test.id
}
resource "aws_security_group" "alb" {
  name_prefix = "${var.name}-alb-"
  vpc_id      = aws_vpc.test.id
  egress {
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = [aws_vpc.test.cidr_block]
  }
}
resource "aws_security_group" "tasks" {
  name_prefix = "${var.name}-tasks-"
  vpc_id      = aws_vpc.test.id
  egress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
resource "aws_lb" "test" {
  name               = var.name
  load_balancer_type = "application"
  ip_address_type    = "ipv4"
  subnets            = aws_subnet.test[*].id
  security_groups    = [aws_security_group.alb.id]
}
resource "aws_lb_target_group" "blue" {
  for_each             = local.strategies
  name                 = "${var.name}-${each.key}-b"
  port                 = 8080
  protocol             = "HTTP"
  target_type          = "ip"
  vpc_id               = aws_vpc.test.id
  deregistration_delay = 10
  health_check {
    path                = "/health"
    interval            = 5
    timeout             = 2
    healthy_threshold   = 2
    unhealthy_threshold = 2
  }
}
resource "aws_lb_target_group" "green" {
  for_each             = local.strategies
  name                 = "${var.name}-${each.key}-g"
  port                 = 8080
  protocol             = "HTTP"
  target_type          = "ip"
  vpc_id               = aws_vpc.test.id
  deregistration_delay = 10
  health_check {
    path                = "/health"
    interval            = 5
    timeout             = 2
    healthy_threshold   = 2
    unhealthy_threshold = 2
  }
}
resource "aws_lb_listener" "production" {
  load_balancer_arn = aws_lb.test.arn
  port              = 80
  protocol          = "HTTP"
  default_action {
    type = "fixed-response"
    fixed_response {
      content_type = "text/plain"
      status_code  = "404"
    }
  }
}
resource "aws_lb_listener_rule" "production" {
  for_each     = local.strategies
  listener_arn = aws_lb_listener.production.arn
  priority     = index(sort(keys(local.strategies)), each.key) + 1
  action {
    type = "forward"
    forward {
      target_group {
        arn    = aws_lb_target_group.blue[each.key].arn
        weight = 100
      }
      target_group {
        arn    = aws_lb_target_group.green[each.key].arn
        weight = 0
      }
    }
  }
  condition {
    path_pattern { values = ["/${each.key}/*"] }
  }
  lifecycle {
    # ECS owns routing once bootstrap completes.
    ignore_changes = [action]
  }
}
resource "aws_ecs_cluster" "test" {
  name = var.name
}
resource "aws_cloudwatch_log_group" "test" {
  name              = "/tenet/integration/${var.name}"
  retention_in_days = 3
}
resource "aws_cloudwatch_metric_alarm" "failure" {
  for_each            = local.strategies
  alarm_name          = "${var.name}-${each.key}-failure"
  namespace           = "Tenet/Integration"
  metric_name         = "Failure"
  dimensions          = { Run = var.name, Strategy = each.key }
  comparison_operator = "GreaterThanOrEqualToThreshold"
  threshold           = 1
  evaluation_periods  = 1
  period              = 60
  statistic           = "Maximum"
  treat_missing_data  = "notBreaching"
}
resource "aws_ecs_task_definition" "baseline" {
  for_each                 = local.strategies
  family                   = "${var.name}-${each.key}"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.execution.arn
  runtime_platform {
    cpu_architecture        = "X86_64"
    operating_system_family = "LINUX"
  }
  container_definitions = jsonencode([{
    name         = "web", image = var.image, essential = true
    portMappings = [{ containerPort = 8080, protocol = "tcp" }]
    healthCheck = {
      command  = ["CMD", "python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8080/health', timeout=2)"]
      interval = 5, timeout = 3, retries = 2, startPeriod = 10
    }
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group  = aws_cloudwatch_log_group.test.name
        awslogs-region = var.region, awslogs-stream-prefix = each.key
      }
    }
  }])
}
resource "aws_ecs_service" "test" {
  for_each        = local.strategies
  name            = "${var.name}-${each.key}"
  cluster         = aws_ecs_cluster.test.arn
  task_definition = aws_ecs_task_definition.baseline[each.key].arn
  desired_count   = 2
  launch_type     = "FARGATE"
  deployment_controller { type = "ECS" }
  deployment_configuration {
    strategy             = each.value
    bake_time_in_minutes = 2
    dynamic "canary_configuration" {
      for_each = each.key == "canary" ? [1] : []
      content {
        canary_percent              = 25
        canary_bake_time_in_minutes = 2
      }
    }
    dynamic "linear_configuration" {
      for_each = each.key == "linear" ? [1] : []
      content {
        step_percent              = 25
        step_bake_time_in_minutes = 2
      }
    }
  }
  alarms {
    enable      = true
    rollback    = false
    alarm_names = [aws_cloudwatch_metric_alarm.failure[each.key].alarm_name]
  }
  network_configuration {
    subnets          = aws_subnet.test[*].id
    security_groups  = [aws_security_group.tasks.id]
    assign_public_ip = true
  }
  load_balancer {
    container_name   = "web"
    container_port   = 8080
    target_group_arn = aws_lb_target_group.blue[each.key].arn
    advanced_configuration {
      alternate_target_group_arn = aws_lb_target_group.green[each.key].arn
      production_listener_rule   = aws_lb_listener_rule.production[each.key].arn
      role_arn                   = aws_iam_role.routing.arn
    }
  }
  lifecycle {
    # Tenet owns deployments; reapplying bootstrap must not revert a test.
    ignore_changes = [task_definition, desired_count, deployment_configuration, alarms, load_balancer]
  }
  depends_on = [aws_iam_role_policy_attachment.routing, aws_iam_role_policy_attachment.execution, aws_route_table_association.test]
}

resource "aws_vpc_security_group_ingress_rule" "operator" {
  security_group_id = aws_security_group.alb.id
  cidr_ipv4         = var.operator_cidr
  from_port         = 80
  to_port           = 80
  ip_protocol       = "tcp"
}
resource "aws_vpc_security_group_ingress_rule" "alb_to_tasks" {
  security_group_id            = aws_security_group.tasks.id
  referenced_security_group_id = aws_security_group.alb.id
  from_port                    = 8080
  to_port                      = 8080
  ip_protocol                  = "tcp"
}
