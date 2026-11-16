variable "name_prefix" {
  description = "Prefix used for AWS resource names."
  type        = string
  default     = "tenet"
}

variable "vpc_cidr" {
  description = "CIDR block for the bootstrap VPC."
  type        = string
  default     = "10.42.0.0/16"
}

variable "operator_cidr_blocks" {
  description = "Explicit IPv4 CIDRs allowed to reach the HTTPS ALB; unrestricted ingress is rejected."
  type        = list(string)
  validation {
    condition = length(var.operator_cidr_blocks) > 0 && alltrue([
      for cidr in var.operator_cidr_blocks : can(cidrnetmask(cidr)) && try(tonumber(split("/", cidr)[1]) > 0, false)
    ])
    error_message = "Set nonempty, valid IPv4 operator CIDRs with a prefix longer than /0."
  }
}

variable "repo_url" {
  description = "Git clone URL that the repo-sync sidecars keep mounted into both Tenet tasks. Public HTTPS clone URLs work anonymously; private HTTPS repos can use repo_sync_token_secret_arn."
  type        = string
  default     = "https://github.com/DevopsArtFactory/tenet.git"
}

variable "repo_sync_token_secret_arn" {
  description = "Optional AWS Secrets Manager secret ARN containing an HTTPS token for the repo-sync sidecars. Leave null to keep anonymous public clone behavior."
  type        = string
  default     = null

  validation {
    condition     = var.repo_sync_token_secret_arn == null ? true : can(regex("^arn:[^:]+:secretsmanager:", var.repo_sync_token_secret_arn))
    error_message = "repo_sync_token_secret_arn must be null or an AWS Secrets Manager secret ARN."
  }
}

variable "repo_branch" {
  description = "Git branch or ref that the repo-sync sidecars keep checked out."
  type        = string
  default     = "main"
}

variable "repo_sync_interval_seconds" {
  description = "How often the repo-sync sidecars fetch and hard-reset to the configured branch."
  type        = number
  default     = 30
}

variable "image_source_path" {
  description = "Path to the Tenet repository root used by terraform apply to docker build and push the runtime image."
  type        = string
  default     = "../../.."
}

variable "image_tag" {
  description = "Tag pushed to ECR for the built Tenet image."
  type        = string
  default     = "bootstrap"
}

variable "api_cpu" {
  description = "Fargate CPU units for the tenet-api task."
  type        = number
  default     = 512
}

variable "api_memory" {
  description = "Fargate memory (MiB) for the tenet-api task."
  type        = number
  default     = 1024
}

variable "controller_cpu" {
  description = "Fargate CPU units for the tenet-controller task."
  type        = number
  default     = 512
}

variable "controller_memory" {
  description = "Fargate memory (MiB) for the tenet-controller task."
  type        = number
  default     = 1024
}

variable "api_desired_count" {
  description = "Desired task count for the tenet-api ECS service."
  type        = number
  default     = 1
}

variable "controller_desired_count" {
  description = "Desired task count for the tenet-controller ECS service."
  type        = number
  default     = 1
}

variable "log_retention_days" {
  description = "CloudWatch Logs retention in days."
  type        = number
  default     = 14
}

variable "controller_requeue_seconds" {
  description = "TENET_CONTROLLER_REQUEUE_SECONDS for the ECS controller service."
  type        = number
  default     = 30
}

variable "controller_workers" {
  description = "TENET_CONTROLLER_WORKERS for the ECS controller service."
  type        = number
  default     = 1
}

variable "api_request_timeout" {
  description = "TENET_API_REQUEST_TIMEOUT for the ECS API service."
  type        = string
  default     = "30s"
}

variable "observer_role_arns" {
  description = "Target-account observer roles the Tenet runtime may assume for cross-account read-only inspection."
  type        = list(string)
  default     = []
}

variable "tags" {
  description = "Common resource tags."
  type        = map(string)
  default = {
    ManagedBy = "terraform"
    Project   = "tenet"
  }
}

variable "alb_internal" {
  description = "Keep the ALB private by default. Operators need VPN or equivalent VPC connectivity."
  type        = bool
  default     = true
}

variable "hostname" {
  description = "DNS hostname covered by the ACM certificate; point it at the ALB after deployment."
  type        = string
  validation {
    condition     = can(regex("^[a-zA-Z0-9][a-zA-Z0-9.-]*[a-zA-Z0-9]$", var.hostname)) && strcontains(var.hostname, ".")
    error_message = "Set a DNS hostname without scheme, port, or path."
  }
}

variable "certificate_arn" {
  description = "Existing ACM certificate ARN for the HTTPS listener in the deployment region."
  type        = string
  validation {
    condition     = can(regex("^arn:[^:]+:acm:[^:]+:[0-9]{12}:certificate/", var.certificate_arn))
    error_message = "An ACM certificate ARN is required."
  }
}

variable "github_oauth_client_id" {
  description = "GitHub OAuth application client ID; callback is https://hostname/auth/callback/github."
  type        = string
  validation {
    condition     = length(trimspace(var.github_oauth_client_id)) > 0
    error_message = "A GitHub OAuth client ID is required."
  }
}

variable "github_oauth_client_secret_arn" {
  description = "Secrets Manager ARN containing the GitHub OAuth client secret as a plain string."
  type        = string
  validation {
    condition     = can(regex("^arn:[^:]+:secretsmanager:[^:]+:[0-9]{12}:secret:", var.github_oauth_client_secret_arn))
    error_message = "An OAuth client secret ARN is required."
  }
}

variable "auth_session_secret_arn" {
  description = "Secrets Manager ARN containing a cryptographically random session signing secret (at least 32 bytes)."
  type        = string
  validation {
    condition     = can(regex("^arn:[^:]+:secretsmanager:[^:]+:[0-9]{12}:secret:", var.auth_session_secret_arn))
    error_message = "A session signing secret ARN is required."
  }
}
