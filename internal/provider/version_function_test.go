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
				// Check that the provider version is "test" (as set in testAccProtoV6ProviderFactories)
				resource.TestCheckOutput("provider_version", "test"),
				// Check that apko version is not empty and looks like a version
				resource.TestMatchOutput("apko_version", regexp.MustCompile(`^v\d+\.\d+\.\d+.*|unknown$`)),
			),
		}},
	})
}
