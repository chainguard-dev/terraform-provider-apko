package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccVersionFunction(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
locals {
  version_info = provider::apko::version()
}

output "provider_version" {
  value = local.version_info.provider_version
}

output "apko_version" {
  value = local.version_info.apko_version
}`,
			Check: resource.ComposeTestCheckFunc(
				// Check that the provider version is not empty and looks like a version
				// With build info, it should be something like "v0.23.1-0.20250729145754-7f4b7167d62c" or "test"
				resource.TestMatchOutput("provider_version", regexp.MustCompile(`^(v\d+\.\d+\.\d+.*|dev)$`)),
				// Check that apko version is not empty and looks like a version
				resource.TestMatchOutput("apko_version", regexp.MustCompile(`^v\d+\.\d+\.\d+.*|unknown$`)),
			),
		}},
	})
}
