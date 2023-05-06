package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDataSourceConfig(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
  archs:
  - amd64
  - aarch64
  EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.#", "2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "x86_64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.1", "aarch64"),
			),
		}, {
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
  archs:
  - x86_64
  - arm64
  EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.#", "2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "x86_64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.1", "aarch64"),
			),
		}},
	})
}

func TestAccDataSourceConfig_ProviderOpts(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories: []string{"https://packages.wolfi.dev/os"},
				keyring:      []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:        []string{"x86_64", "aarch64"},
			}),
		},
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
contents:
  packages:
    - wolfi-baselayout
    - ca-certificates-bundle
    - tzdata
  EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.#", "2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "x86_64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.1", "aarch64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.#", "3"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.0", "wolfi-baselayout"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.1", "ca-certificates-bundle"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.2", "tzdata"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.repositories.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.repositories.0", "https://packages.wolfi.dev/os"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.keyring.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.keyring.0", "https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"),
			),
		}},
	})
}
