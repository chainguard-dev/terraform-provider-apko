package provider

import (
	"regexp"
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
contents:
  repositories:
  - ./packages
archs:
- amd64
- aarch64
EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.#", "2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "amd64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.1", "arm64"),
			),
		}, {
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
contents:
  repositories:
  - ./packages
archs:
- x86_64
- arm64
EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.#", "2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "amd64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.1", "arm64"),
			),
		}},
	})
}

func TestAccDataSourceConfig_ExtraPackages(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories:       []string{"https://packages.wolfi.dev/os"},
				buildRespositories: []string{"./packages"},
				keyring:            []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:              []string{"x86_64", "aarch64"},
				packages:           []string{"wolfi-baselayout=20230201-r0"},
				anns: map[string]string{
					"bar": "provider-provided",
					"baz": "provider-provided",
				},
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
annotations:
  bar: config-provided
EOF
  extra_packages = ["tzdata=2023c-r0"]
  default_annotations = {
	foo: "bar"
	bar: "datasource-provided"
  }
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.#", "4"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.0", "ca-certificates-bundle=20230506-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.1", "glibc-locale-posix=2.37-r6"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.2", "tzdata=2023c-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.3", "wolfi-baselayout=20230201-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.annotations.%", "3"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.annotations.foo", "bar"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.annotations.bar", "config-provided"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.annotations.baz", "provider-provided"),
			),
		}},
	})
}

func TestAccDataSourceConfig_ProviderOpts_Locked(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories:       []string{"https://packages.wolfi.dev/os"},
				buildRespositories: []string{"./packages"},
				keyring:            []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:              []string{"x86_64", "aarch64"},
				packages:           []string{"wolfi-baselayout=20230201-r0"},
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
    - tzdata=2023c-r0
  EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.#", "2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "amd64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.1", "arm64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.#", "4"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.0", "ca-certificates-bundle=20230506-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.1", "glibc-locale-posix=2.37-r6"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.2", "tzdata=2023c-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.3", "wolfi-baselayout=20230201-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.repositories.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.repositories.0", "https://packages.wolfi.dev/os"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.keyring.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.keyring.0", "https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"),
			),
		}},
	})
}

func TestAccDataSourceConfig_ProviderOpts_Unlocked(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories:       []string{"https://packages.wolfi.dev/os"},
				buildRespositories: []string{"./packages"},
				keyring:            []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:              []string{"x86_64", "aarch64"},
				packages:           []string{"wolfi-baselayout"},
			}),
		},
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
contents:
  packages:
    - tzdata
  EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.#", "2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "amd64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.1", "arm64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.#", "3"),
				// This is pulled in as a transitive dependency of wolfi-baselayout.
				resource.TestMatchResourceAttr("data.apko_config.this", "config.contents.packages.0", regexp.MustCompile("^ca-certificates-bundle=.*")),
				resource.TestMatchResourceAttr("data.apko_config.this", "config.contents.packages.1", regexp.MustCompile("^tzdata=.*")),
				resource.TestMatchResourceAttr("data.apko_config.this", "config.contents.packages.2", regexp.MustCompile("^wolfi-baselayout=.*")),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.repositories.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.repositories.0", "https://packages.wolfi.dev/os"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.keyring.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.keyring.0", "https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"),
			),
		}},
	})
}

func TestAccDataSourceConfig_ProviderOpts_OverrideArchitecture(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories:       []string{"https://packages.wolfi.dev/os"},
				buildRespositories: []string{"./packages"},
				keyring:            []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:              []string{"x86_64", "aarch64"},
				packages:           []string{"wolfi-baselayout"},
			}),
		},
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
archs:
  - aarch64
contents:
  packages:
    - tzdata
  EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "arm64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.#", "3"),
				// This is pulled in as a transitive dependency of wolfi-baselayout.
				resource.TestMatchResourceAttr("data.apko_config.this", "config.contents.packages.0", regexp.MustCompile("^ca-certificates-bundle=.*")),
				resource.TestMatchResourceAttr("data.apko_config.this", "config.contents.packages.1", regexp.MustCompile("^tzdata=.*")),
				resource.TestMatchResourceAttr("data.apko_config.this", "config.contents.packages.2", regexp.MustCompile("^wolfi-baselayout=.*")),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.repositories.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.repositories.0", "https://packages.wolfi.dev/os"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.keyring.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.keyring.0", "https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"),
			),
		}},
	})
}

func TestAccDataSourceConfig_Invalid(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
contents:
  repositories:
  - ./packages

unknown-field: 'blah'
EOF
}`,
			ExpectError: regexp.MustCompile("field unknown-field not found in type types.ImageConfiguration"),
		}},
	})
}
