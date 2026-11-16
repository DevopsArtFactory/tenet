output "alb_dns_name" {
  description = "DNS name for the Tenet ALB."
  value       = aws_lb.api.dns_name
}

output "tenet_url" {
  description = "HTTPS entrypoint; configure this hostname to resolve to the ALB."
  value       = "https://${var.hostname}"
}

output "ecr_repository_url" {
  description = "ECR repository that receives the Terraform-built Tenet runtime image."
  value       = aws_ecr_repository.tenet.repository_url
}

output "runtime_role_arn" {
  description = "IAM role ARN used by both Tenet ECS tasks; trust this role from target-account observer roles for cross-account reads."
  value       = aws_iam_role.runtime.arn
}

output "observer_role_arns" {
  description = "Observer roles this bootstrap runtime is allowed to assume."
  value       = var.observer_role_arns
}

output "ecs_cluster_name" {
  description = "Name of the ECS cluster hosting Tenet."
  value       = aws_ecs_cluster.this.name
}
