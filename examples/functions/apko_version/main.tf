terraform {
  required_providers {
    apko = {
      source = "chainguard-dev/apko"
    }
  }
}

locals {
  version_info = provider::apko::version()
}

output "provider_version" {
  description = "The version of the terraform-provider-apko"
  value       = local.version_info.provider_version
}

output "apko_version" {
  description = "The version of the underlying apko package"
  value       = local.version_info.apko_version
}

output "all_version_info" {
  description = "Complete version information object"
  value       = local.version_info
}
