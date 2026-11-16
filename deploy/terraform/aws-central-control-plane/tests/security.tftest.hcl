# Plan-only, mocked providers: never builds images or calls AWS.
mock_provider "aws" {
  mock_data "aws_availability_zones" {
    defaults = { names = ["ap-northeast-2a", "ap-northeast-2c"] }
  }
  mock_data "aws_region" {
    defaults = { name = "ap-northeast-2" }
  }
}
override_resource {
  target          = aws_ecr_repository.tenet
  override_during = plan
  values          = { repository_url = "123456789012.dkr.ecr.ap-northeast-2.amazonaws.com/tenet" }
}
override_resource {
  target          = aws_elasticache_cluster.redis
  override_during = plan
  values          = { cache_nodes = [{ id = "0001", address = "redis.example.test", port = 6379, availability_zone = "ap-northeast-2a" }] }
}
variables {
  operator_cidr_blocks           = ["10.20.0.0/16"]
  hostname                       = "tenet.example.com"
  certificate_arn                = "arn:aws:acm:ap-northeast-2:123456789012:certificate/00000000-0000-0000-0000-000000000000"
  github_oauth_client_id         = "test-client"
  github_oauth_client_secret_arn = "arn:aws:secretsmanager:ap-northeast-2:123456789012:secret:oauth-test"
  auth_session_secret_arn        = "arn:aws:secretsmanager:ap-northeast-2:123456789012:secret:session-test"
}
run "secure_bootstrap" {
  command = plan
  assert {
    condition     = aws_lb.api.internal && aws_lb_listener.https.protocol == "HTTPS" && aws_lb_listener.https.port == 443 && aws_lb_listener.https.certificate_arn == var.certificate_arn
    error_message = "Ingress must default to private HTTPS with the supplied certificate."
  }
  assert {
    condition     = alltrue([for rule in aws_security_group.alb.ingress : rule.from_port == 443 && rule.to_port == 443 && toset(rule.cidr_blocks) == toset(var.operator_cidr_blocks)])
    error_message = "ALB ingress must be restricted to explicit operator CIDRs on HTTPS."
  }
  assert {
    condition     = alltrue([for name in ["TENET_API_ENFORCE_AUTHENTICATED_IDENTITY", "TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION", "TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION", "TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY", "TENET_AUTH_SESSION_COOKIE_SECURE"] : lookup({ for env in jsondecode(local.api_container_definitions)[1].environment : env.name => env.value }, name, "false") == "true"])
    error_message = "Managed login, RBAC and secure cookies must be enforced."
  }
  assert {
    condition     = length(jsondecode(local.api_container_definitions)[1].secrets) == 2 && jsondecode(aws_iam_role_policy.execution_auth_secrets.policy).Statement[0].Resource == [var.github_oauth_client_secret_arn, var.auth_session_secret_arn]
    error_message = "OAuth secrets must come from explicitly scoped Secrets Manager ARNs."
  }
}
run "reject_world_open_ingress" {
  command = plan
  variables { operator_cidr_blocks = ["0.0.0.0/0"] }
  expect_failures = [var.operator_cidr_blocks]
}
run "reject_missing_operator_networks" {
  command = plan
  variables { operator_cidr_blocks = [] }
  expect_failures = [var.operator_cidr_blocks]
}
run "reject_missing_authentication" {
  command = plan
  variables { github_oauth_client_id = "" }
  expect_failures = [var.github_oauth_client_id]
}
