package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLayeringStrategyValidation verifies that the layering strategy validation works.
func TestLayeringStrategyValidation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(New("test")()),
		},
		Steps: []resource.TestStep{
			{
				Config: `
provider "apko" {
  default_layering = {
    strategy = "invalid-strategy"
    budget = 5
  }
}

resource "apko_build" "test" {
  repo = "example.com/test"
  config = jsonencode({
    contents = {
      repositories = ["https://packages.wolfi.dev/os"]
      keyring = ["https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"]
      packages = ["wolfi-baselayout"]
    }
  })
}
`,
				ExpectError: regexp.MustCompile(`Attribute default_layering.strategy value must be one of: \["origin"\]`),
			},
		},
	})
}

// TestLayeringBudgetValidation verifies that the layering budget validation works.
func TestLayeringBudgetValidation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(New("test")()),
		},
		Steps: []resource.TestStep{
			{
				Config: `
provider "apko" {
  default_layering = {
    strategy = "origin"
    budget = 0
  }
}

resource "apko_build" "test" {
  repo = "example.com/test"
  config = jsonencode({
    contents = {
      repositories = ["https://packages.wolfi.dev/os"]
      keyring = ["https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"]
      packages = ["wolfi-baselayout"]
    }
  })
}
`,
				ExpectError: regexp.MustCompile(`Attribute default_layering.budget value must be at least 1`),
			},
		},
	})
}
