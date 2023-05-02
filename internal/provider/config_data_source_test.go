package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDataSourceConfig(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config = <<EOF
  archs:
  - amd64
  - aarch64
  EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr(
					"data.apko_config.this", "data.archs.#", regexp.MustCompile("^2$")),
				resource.TestMatchResourceAttr(
					"data.apko_config.this", "data.archs.0", regexp.MustCompile("^x86_64$")),
				resource.TestMatchResourceAttr(
					"data.apko_config.this", "data.archs.1", regexp.MustCompile("^aarch64$")),
			),
		}, {
			Config: `
data "apko_config" "this" {
  config = <<EOF
  archs:
  - x86_64
  - arm64
  EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr(
					"data.apko_config.this", "data.archs.#", regexp.MustCompile("^2$")),
				resource.TestMatchResourceAttr(
					"data.apko_config.this", "data.archs.0", regexp.MustCompile("^x86_64$")),
				resource.TestMatchResourceAttr(
					"data.apko_config.this", "data.archs.1", regexp.MustCompile("^aarch64$")),
			),
		}},
	})
}
