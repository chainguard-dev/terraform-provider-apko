package provider

import (
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccDataSourceTags(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories: []string{"https://packages.wolfi.dev/os"},
				keyring:      []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:        []string{"x86_64", "aarch64"},
				packages:     []string{"wolfi-baselayout=20230201-r0"},
			}),
		},
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
contents:
  packages:
  - ca-certificates-bundle=20230506-r0
  - glibc-locale-posix=2.37-r6
  - ko=0.13.0-r3
EOF
  extra_packages = ["tzdata=2023c-r0"]
}

data "apko_tags" "ca-certs" {
  config         = data.apko_config.this.config
  target_package = "ca-certificates-bundle"
}

data "apko_tags" "glibc" {
  config         = data.apko_config.this.config
  target_package = "glibc-locale-posix"
}

data "apko_tags" "wolfi-baselayout" {
  config         = data.apko_config.this.config
  target_package = "wolfi-baselayout"
}

data "apko_tags" "tzdata" {
  config         = data.apko_config.this.config
  target_package = "tzdata"
}

data "apko_tags" "ko" {
  config         = data.apko_config.this.config
  target_package = "ko"
}
`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_tags.glibc", "tags.#", "3"),
				resource.TestCheckResourceAttr("data.apko_tags.glibc", "tags.0", "2"),
				resource.TestCheckResourceAttr("data.apko_tags.glibc", "tags.1", "2.37"),
				resource.TestCheckResourceAttr("data.apko_tags.glibc", "tags.2", "2.37-r6"),

				resource.TestCheckResourceAttr("data.apko_tags.ca-certs", "tags.#", "2"),
				resource.TestCheckResourceAttr("data.apko_tags.ca-certs", "tags.0", "20230506"),
				resource.TestCheckResourceAttr("data.apko_tags.ca-certs", "tags.1", "20230506-r0"),

				resource.TestCheckResourceAttr("data.apko_tags.wolfi-baselayout", "tags.#", "2"),
				resource.TestCheckResourceAttr("data.apko_tags.wolfi-baselayout", "tags.0", "20230201"),
				resource.TestCheckFunc(func(s *terraform.State) error {
					ms := s.RootModule()
					rs, ok := ms.Resources["data.apko_config.this"]
					if !ok {
						return errors.New("asdf")
					}
					t.Logf("GOT: %v", rs.Primary.Attributes)
					return nil
				}),
				resource.TestCheckResourceAttr("data.apko_tags.wolfi-baselayout", "tags.1", "20230201-r0"),

				resource.TestCheckResourceAttr("data.apko_tags.tzdata", "tags.#", "2"),
				resource.TestCheckResourceAttr("data.apko_tags.tzdata", "tags.0", "2023c"),
				resource.TestCheckResourceAttr("data.apko_tags.tzdata", "tags.1", "2023c-r0"),

				resource.TestCheckResourceAttr("data.apko_tags.ko", "tags.#", "4"),
				resource.TestCheckResourceAttr("data.apko_tags.ko", "tags.0", "0"),
				resource.TestCheckResourceAttr("data.apko_tags.ko", "tags.1", "0.13"),
				resource.TestCheckResourceAttr("data.apko_tags.ko", "tags.2", "0.13.0"),
				resource.TestCheckResourceAttr("data.apko_tags.ko", "tags.3", "0.13.0-r3"),
			),
		}},
	})
}
