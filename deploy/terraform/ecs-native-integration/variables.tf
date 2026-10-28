variable "account_id" {
  type = string
  validation {
    condition     = can(regex("^[0-9]{12}$", var.account_id))
    error_message = "Select a dedicated 12-digit sandbox AWS account ID."
  }
}
variable "region" {
  type    = string
  default = "ap-northeast-2"
}
variable "availability_zones" {
  type = list(string)
  validation {
    condition     = length(var.availability_zones) == 2 && length(distinct(var.availability_zones)) == 2
    error_message = "Select two distinct availability zones in the selected region."
  }
}
variable "name" {
  type    = string
  default = "tenet-native-e2e"
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{2,18}$", var.name))
    error_message = "Use 3-19 lowercase letters, digits or hyphens, starting with a letter."
  }
}
variable "operator_cidr" {
  type = string
  validation {
    condition     = can(cidrnetmask(var.operator_cidr)) && try(tonumber(split("/", var.operator_cidr)[1]) >= 24, false)
    error_message = "Specify a restricted IPv4 operator CIDR (/24 or narrower)."
  }
}
variable "runner_role_arn" {
  type = string
  validation {
    condition     = can(regex("^arn:aws:iam::[0-9]{12}:role/.+$", var.runner_role_arn))
    error_message = "Specify the existing IAM role used by the isolated Tenet test installation."
  }
}
variable "image" {
  description = "Baseline test image built from tests/ecs-native-integration/app, pinned by digest."
  type        = string
  validation {
    condition     = can(regex("^[^ ]+@sha256:[0-9a-f]{64}$", var.image))
    error_message = "Supply an accessible Linux amd64 image pinned by SHA256 digest."
  }
}
