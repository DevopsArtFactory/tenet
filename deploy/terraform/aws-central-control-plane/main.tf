locals {
  azs                   = slice(data.aws_availability_zones.available.names, 0, 2)
  public_subnet_cidrs   = [for index in range(length(local.azs)) : cidrsubnet(var.vpc_cidr, 8, index)]
  private_subnet_cidrs  = [for index in range(length(local.azs)) : cidrsubnet(var.vpc_cidr, 8, index + 10)]
  common_tags           = merge(var.tags, { NamePrefix = var.name_prefix })
  image_source_abs_path = abspath("${path.module}/${var.image_source_path}")
  image_source_hash = sha256(join("", concat(
    [filesha256("${local.image_source_abs_path}/Dockerfile")],
    [filesha256("${local.image_source_abs_path}/go.mod")],
    [filesha256("${local.image_source_abs_path}/go.sum")],
    [for file in fileset(local.image_source_abs_path, "cmd/**") : filesha256("${local.image_source_abs_path}/${file}")],
    [for file in fileset(local.image_source_abs_path, "internal/**") : filesha256("${local.image_source_abs_path}/${file}")]
  )))
  image_uri = "${aws_ecr_repository.tenet.repository_url}:${var.image_tag}"

  repo_sync_command = templatefile("${path.module}/templates/repo-sync.sh.tftpl", {
    repo_url      = var.repo_url
    repo_branch   = var.repo_branch
    sync_interval = var.repo_sync_interval_seconds
  })
  repo_sync_token_secrets = var.repo_sync_token_secret_arn == null ? [] : [
    {
      name      = "TENET_REPO_SYNC_TOKEN"
      valueFrom = var.repo_sync_token_secret_arn
    }
  ]

  api_start_command = templatefile("${path.module}/templates/start-api.sh.tftpl", {
    api_request_timeout = var.api_request_timeout
  })

  controller_start_command = templatefile("${path.module}/templates/start-controller.sh.tftpl", {
    controller_workers         = var.controller_workers
    controller_requeue_seconds = var.controller_requeue_seconds
  })

  api_container_definitions = jsonencode([
    merge(
      {
        name       = "repo-sync"
        image      = "public.ecr.aws/docker/library/alpine/git:2.47.2"
        essential  = true
        entryPoint = ["/bin/sh", "-lc"]
        command    = [local.repo_sync_command]
        mountPoints = [
          {
            sourceVolume  = "repo"
            containerPath = "/workspace/repo"
            readOnly      = false
          }
        ]
        logConfiguration = {
          logDriver = "awslogs"
          options = {
            awslogs-group         = aws_cloudwatch_log_group.api.name
            awslogs-region        = data.aws_region.current.name
            awslogs-stream-prefix = "repo-sync"
          }
        }
      },
      length(local.repo_sync_token_secrets) == 0 ? {} : {
        secrets = local.repo_sync_token_secrets
      }
    ),
    {
      name       = "tenet-api"
      image      = local.image_uri
      essential  = true
      entryPoint = ["/bin/sh", "-lc"]
      command    = [local.api_start_command]
      portMappings = [
        {
          containerPort = 8080
          hostPort      = 8080
          protocol      = "tcp"
        }
      ]
      environment = [
        { name = "TENET_API_ADDR", value = ":8080" },
        { name = "TENET_API_TRUST_AUTHENTICATED_HEADERS", value = "false" },
        { name = "TENET_API_ENFORCE_AUTHENTICATED_IDENTITY", value = "true" },
        { name = "TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION", value = "true" },
        { name = "TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION", value = "true" },
        { name = "TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY", value = "true" },
        { name = "TENET_AUTH_PROVIDER", value = "github" },
        { name = "TENET_GITHUB_OAUTH_CLIENT_ID", value = var.github_oauth_client_id },
        { name = "TENET_GITHUB_OAUTH_REDIRECT_URL", value = "https://${var.hostname}/auth/callback/github" },
        { name = "TENET_AUTH_SESSION_COOKIE_SECURE", value = "true" },
        { name = "TENET_AUTH_STATE_STORE_PATH", value = "/workspace/control-plane-state/oauth-states.json" },
        { name = "TENET_AUTH_IDENTITY_STORE_PATH", value = "/workspace/control-plane-state/oauth-identities.json" },
        { name = "TENET_CATALOG_SOURCE", value = "filesystem" },
        { name = "TENET_REPO_ROOT", value = "/workspace/repo" },
        { name = "TENET_STATUS_BACKEND", value = "redis" },
        { name = "TENET_REDIS_ADDR", value = "${aws_elasticache_cluster.redis.cache_nodes[0].address}:${aws_elasticache_cluster.redis.port}" },
        { name = "TENET_API_REQUEST_TIMEOUT", value = var.api_request_timeout },
        { name = "TENET_PLAN_STORE_PATH", value = "/workspace/control-plane-state/plan-previews.json" },
        { name = "TENET_APPLY_STORE_PATH", value = "/workspace/control-plane-state/apply-requests.json" },
        { name = "TENET_IMAGE_SOURCE_HASH", value = local.image_source_hash }
      ]
      secrets = [
        { name = "TENET_GITHUB_OAUTH_CLIENT_SECRET", valueFrom = var.github_oauth_client_secret_arn },
        { name = "TENET_AUTH_SESSION_SECRET", valueFrom = var.auth_session_secret_arn }
      ]
      mountPoints = [
        {
          sourceVolume  = "repo"
          containerPath = "/workspace/repo"
          readOnly      = false
        },
        {
          sourceVolume  = "control-plane-state"
          containerPath = "/workspace/control-plane-state"
          readOnly      = false
        }
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.api.name
          awslogs-region        = data.aws_region.current.name
          awslogs-stream-prefix = "tenet-api"
        }
      }
    }
  ])

  controller_container_definitions = jsonencode([
    merge(
      {
        name       = "repo-sync"
        image      = "public.ecr.aws/docker/library/alpine/git:2.47.2"
        essential  = true
        entryPoint = ["/bin/sh", "-lc"]
        command    = [local.repo_sync_command]
        mountPoints = [
          {
            sourceVolume  = "repo"
            containerPath = "/workspace/repo"
            readOnly      = false
          }
        ]
        logConfiguration = {
          logDriver = "awslogs"
          options = {
            awslogs-group         = aws_cloudwatch_log_group.controller.name
            awslogs-region        = data.aws_region.current.name
            awslogs-stream-prefix = "repo-sync"
          }
        }
      },
      length(local.repo_sync_token_secrets) == 0 ? {} : {
        secrets = local.repo_sync_token_secrets
      }
    ),
    {
      name       = "tenet-controller"
      image      = local.image_uri
      essential  = true
      entryPoint = ["/bin/sh", "-lc"]
      command    = [local.controller_start_command]
      environment = [
        { name = "TENET_CATALOG_SOURCE", value = "filesystem" },
        { name = "TENET_REPO_ROOT", value = "/workspace/repo" },
        { name = "TENET_STATUS_BACKEND", value = "redis" },
        { name = "TENET_REDIS_ADDR", value = "${aws_elasticache_cluster.redis.cache_nodes[0].address}:${aws_elasticache_cluster.redis.port}" },
        { name = "TENET_CONTROLLER_WORKERS", value = tostring(var.controller_workers) },
        { name = "TENET_CONTROLLER_REQUEUE_SECONDS", value = tostring(var.controller_requeue_seconds) },
        { name = "TENET_IMAGE_SOURCE_HASH", value = local.image_source_hash }
      ]
      mountPoints = [
        {
          sourceVolume  = "repo"
          containerPath = "/workspace/repo"
          readOnly      = false
        }
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.controller.name
          awslogs-region        = data.aws_region.current.name
          awslogs-stream-prefix = "tenet-controller"
        }
      }
    }
  ])
}

data "aws_region" "current" {}

data "aws_caller_identity" "current" {}

data "aws_availability_zones" "available" {
  state = "available"
}

resource "aws_vpc" "this" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = merge(local.common_tags, { Name = "${var.name_prefix}-vpc" })
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id

  tags = merge(local.common_tags, { Name = "${var.name_prefix}-igw" })
}

resource "aws_subnet" "public" {
  for_each = {
    for index, az in local.azs : az => {
      cidr_block = local.public_subnet_cidrs[index]
      az         = az
    }
  }

  vpc_id                  = aws_vpc.this.id
  cidr_block              = each.value.cidr_block
  availability_zone       = each.value.az
  map_public_ip_on_launch = true

  tags = merge(local.common_tags, {
    Name = "${var.name_prefix}-public-${replace(each.key, data.aws_region.current.name, "")}"
    Tier = "public"
  })
}

resource "aws_subnet" "private" {
  for_each = {
    for index, az in local.azs : az => {
      cidr_block = local.private_subnet_cidrs[index]
      az         = az
    }
  }

  vpc_id            = aws_vpc.this.id
  cidr_block        = each.value.cidr_block
  availability_zone = each.value.az

  tags = merge(local.common_tags, {
    Name = "${var.name_prefix}-private-${replace(each.key, data.aws_region.current.name, "")}"
    Tier = "private"
  })
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.this.id
  }

  tags = merge(local.common_tags, { Name = "${var.name_prefix}-public-rt" })
}

resource "aws_route_table_association" "public" {
  for_each       = aws_subnet.public
  subnet_id      = each.value.id
  route_table_id = aws_route_table.public.id
}

resource "aws_security_group" "alb" {
  name        = "${var.name_prefix}-alb"
  description = "Ingress to the Tenet ALB"
  vpc_id      = aws_vpc.this.id

  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = var.operator_cidr_blocks
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, { Name = "${var.name_prefix}-alb-sg" })
}

resource "aws_security_group" "tasks" {
  name        = "${var.name_prefix}-tasks"
  description = "Tenet ECS tasks"
  vpc_id      = aws_vpc.this.id

  ingress {
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, { Name = "${var.name_prefix}-tasks-sg" })
}

resource "aws_security_group" "redis" {
  name        = "${var.name_prefix}-redis"
  description = "Redis access from Tenet ECS tasks"
  vpc_id      = aws_vpc.this.id

  ingress {
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [aws_security_group.tasks.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, { Name = "${var.name_prefix}-redis-sg" })
}

resource "aws_security_group" "efs" {
  name        = "${var.name_prefix}-efs"
  description = "EFS access from Tenet ECS tasks"
  vpc_id      = aws_vpc.this.id

  ingress {
    from_port       = 2049
    to_port         = 2049
    protocol        = "tcp"
    security_groups = [aws_security_group.tasks.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, { Name = "${var.name_prefix}-efs-sg" })
}

resource "aws_lb" "api" {
  name               = "${var.name_prefix}-alb"
  load_balancer_type = "application"
  internal           = var.alb_internal
  security_groups    = [aws_security_group.alb.id]
  subnets            = var.alb_internal ? values(aws_subnet.private)[*].id : values(aws_subnet.public)[*].id

  tags = merge(local.common_tags, { Name = "${var.name_prefix}-alb" })
}

resource "aws_lb_target_group" "api" {
  name        = substr("${var.name_prefix}-api-tg", 0, 32)
  port        = 8080
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = aws_vpc.this.id

  health_check {
    enabled             = true
    path                = "/healthz"
    healthy_threshold   = 2
    unhealthy_threshold = 2
    interval            = 30
    timeout             = 5
    matcher             = "200"
  }

  tags = merge(local.common_tags, { Name = "${var.name_prefix}-api-tg" })
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.api.arn
  port              = 443
  protocol          = "HTTPS"
  certificate_arn   = var.certificate_arn
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }
}

resource "aws_cloudwatch_log_group" "api" {
  name              = "/aws/ecs/${var.name_prefix}-api"
  retention_in_days = var.log_retention_days

  tags = local.common_tags
}

resource "aws_cloudwatch_log_group" "controller" {
  name              = "/aws/ecs/${var.name_prefix}-controller"
  retention_in_days = var.log_retention_days

  tags = local.common_tags
}

resource "aws_ecr_repository" "tenet" {
  name                 = "${var.name_prefix}/tenet"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = local.common_tags
}

resource "aws_efs_file_system" "state" {
  creation_token = "${var.name_prefix}-state"
  encrypted      = true

  tags = merge(local.common_tags, { Name = "${var.name_prefix}-state" })
}

resource "aws_efs_access_point" "state" {
  file_system_id = aws_efs_file_system.state.id

  posix_user {
    uid = 10001
    gid = 10001
  }

  root_directory {
    path = "/tenet-state"

    creation_info {
      owner_uid   = 10001
      owner_gid   = 10001
      permissions = "0755"
    }
  }

  tags = merge(local.common_tags, { Name = "${var.name_prefix}-state-ap" })
}

resource "aws_efs_mount_target" "state" {
  for_each = aws_subnet.private

  file_system_id  = aws_efs_file_system.state.id
  subnet_id       = each.value.id
  security_groups = [aws_security_group.efs.id]
}

resource "aws_elasticache_subnet_group" "redis" {
  name       = "${var.name_prefix}-redis"
  subnet_ids = values(aws_subnet.private)[*].id

  tags = local.common_tags
}

resource "aws_elasticache_cluster" "redis" {
  cluster_id           = substr("${var.name_prefix}-redis", 0, 20)
  engine               = "redis"
  node_type            = "cache.t4g.micro"
  num_cache_nodes      = 1
  parameter_group_name = "default.redis7"
  engine_version       = "7.1"
  port                 = 6379
  subnet_group_name    = aws_elasticache_subnet_group.redis.name
  security_group_ids   = [aws_security_group.redis.id]

  tags = local.common_tags
}

resource "aws_ecs_cluster" "this" {
  name = "${var.name_prefix}-cluster"

  setting {
    name  = "containerInsights"
    value = "enabled"
  }

  tags = local.common_tags
}

resource "aws_iam_role" "execution" {
  name = "${var.name_prefix}-ecs-execution"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ecs-tasks.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })

  tags = local.common_tags
}

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "execution_repo_sync_secret" {
  count = var.repo_sync_token_secret_arn == null ? 0 : 1

  name = "${var.name_prefix}-repo-sync-secret"
  role = aws_iam_role.execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["secretsmanager:GetSecretValue"]
      Resource = var.repo_sync_token_secret_arn
    }]
  })
}

resource "aws_iam_role" "runtime" {
  name = "${var.name_prefix}-runtime"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ecs-tasks.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })

  tags = local.common_tags
}

resource "aws_iam_role_policy" "execution_auth_secrets" {
  name = "${var.name_prefix}-auth-secrets"
  role = aws_iam_role.execution.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["secretsmanager:GetSecretValue"]
      Resource = [var.github_oauth_client_secret_arn, var.auth_session_secret_arn]
    }]
  })
}

resource "aws_iam_role_policy" "runtime_assume_observer" {
  name = "${var.name_prefix}-runtime-extra"
  role = aws_iam_role.runtime.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [{
        Effect = "Allow"
        Action = [
          "elasticfilesystem:ClientMount",
          "elasticfilesystem:ClientWrite",
          "elasticfilesystem:ClientRootAccess"
        ]
        Resource = [
          aws_efs_file_system.state.arn,
          aws_efs_access_point.state.arn
        ]
      }],
      length(var.observer_role_arns) == 0 ? [] : [{
        Effect   = "Allow"
        Action   = ["sts:AssumeRole"]
        Resource = var.observer_role_arns
      }]
    )
  })
}

resource "terraform_data" "image_build" {
  triggers_replace = {
    image_source_hash = local.image_source_hash
    image_uri         = local.image_uri
    source_path       = local.image_source_abs_path
  }

  provisioner "local-exec" {
    command = <<-EOT
      set -euo pipefail
      aws ecr get-login-password --region ${data.aws_region.current.name} | docker login --username AWS --password-stdin ${aws_ecr_repository.tenet.repository_url}
      docker buildx build --platform linux/amd64 --load -t ${local.image_uri} ${local.image_source_abs_path}
      docker push ${local.image_uri}
    EOT
  }
}

resource "aws_ecs_task_definition" "api" {
  family                   = "${var.name_prefix}-api"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = tostring(var.api_cpu)
  memory                   = tostring(var.api_memory)
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.runtime.arn
  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "X86_64"
  }

  volume {
    name = "repo"
  }

  volume {
    name = "control-plane-state"

    efs_volume_configuration {
      file_system_id     = aws_efs_file_system.state.id
      transit_encryption = "ENABLED"
      authorization_config {
        access_point_id = aws_efs_access_point.state.id
        iam             = "ENABLED"
      }
    }
  }

  container_definitions = local.api_container_definitions

  depends_on = [terraform_data.image_build, aws_iam_role_policy_attachment.execution, aws_iam_role_policy.execution_auth_secrets, aws_iam_role_policy.execution_repo_sync_secret, aws_efs_mount_target.state]

  tags = local.common_tags
}

resource "aws_ecs_task_definition" "controller" {
  family                   = "${var.name_prefix}-controller"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = tostring(var.controller_cpu)
  memory                   = tostring(var.controller_memory)
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.runtime.arn
  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "X86_64"
  }

  volume {
    name = "repo"
  }

  container_definitions = local.controller_container_definitions

  depends_on = [terraform_data.image_build, aws_iam_role_policy_attachment.execution, aws_iam_role_policy.execution_auth_secrets, aws_iam_role_policy.execution_repo_sync_secret]

  tags = local.common_tags
}

resource "aws_ecs_service" "api" {
  name            = "${var.name_prefix}-api"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.api.arn
  desired_count   = var.api_desired_count
  launch_type     = "FARGATE"

  enable_execute_command = true
  force_new_deployment   = true

  network_configuration {
    subnets          = values(aws_subnet.public)[*].id
    security_groups  = [aws_security_group.tasks.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.api.arn
    container_name   = "tenet-api"
    container_port   = 8080
  }

  depends_on = [aws_lb_listener.https]

  tags = local.common_tags
}

resource "aws_ecs_service" "controller" {
  name            = "${var.name_prefix}-controller"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.controller.arn
  desired_count   = var.controller_desired_count
  launch_type     = "FARGATE"

  enable_execute_command = true
  force_new_deployment   = true

  network_configuration {
    subnets          = values(aws_subnet.public)[*].id
    security_groups  = [aws_security_group.tasks.id]
    assign_public_ip = true
  }

  tags = local.common_tags
}
