variable "subscription_id" {
  type        = string
  description = "Azure subscription ID. Supplied by the caller, never committed."
}

variable "location" {
  type    = string
  default = "eastus"
}

variable "prefix" {
  type    = string
  default = "obsaiops"
}

variable "profile" {
  type        = string
  description = "azure-minimal or azure-production"
  default     = "azure-minimal"

  validation {
    condition     = contains(["azure-minimal", "azure-production"], var.profile)
    error_message = "profile must be azure-minimal or azure-production."
  }
}

variable "alert_email" {
  type        = string
  description = "Action group email. Use a shared mailbox, not a personal secret."
  default     = "sre@example.com"
}

variable "tags" {
  type = map(string)
  default = {
    product = "cloud-observability-aiops-platform"
    managed = "terraform"
  }
}

locals {
  minimal     = var.profile == "azure-minimal"
  production  = var.profile == "azure-production"
  enable_aks  = local.production
  enable_net  = local.production
  enable_graf = local.production
}
