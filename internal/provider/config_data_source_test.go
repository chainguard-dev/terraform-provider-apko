package provider

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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
				packages:           []string{"wolfi-baselayout=20230201-r24"},
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
  - ca-certificates-bundle=20250911-r0
  - glibc-locale-posix=2.42-r2
annotations:
  bar: config-provided
EOF
  extra_packages = ["tzdata=2025b-r2"]
  default_annotations = {
	foo: "bar"
	bar: "datasource-provided"
  }
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.#", "4"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.0", "ca-certificates-bundle=20250911-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.1", "glibc-locale-posix=2.42-r2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.2", "tzdata=2025b-r2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.3", "wolfi-baselayout=20230201-r24"),
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
				packages:           []string{"wolfi-baselayout=20230201-r24"},
			}),
		},
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
contents:
  packages:
    - ca-certificates-bundle=20250911-r0
    - glibc-locale-posix=2.42-r2
    - tzdata=2025b-r2
  EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.#", "2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "amd64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.1", "arm64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.#", "4"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.0", "ca-certificates-bundle=20250911-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.1", "glibc-locale-posix=2.42-r2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.2", "tzdata=2025b-r2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.3", "wolfi-baselayout=20230201-r24"),
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

func TestAccDataSourceConfig_ResolvedPackages(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories:       []string{"https://packages.wolfi.dev/os"},
				buildRespositories: []string{"./packages"},
				keyring:            []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:              []string{"x86_64", "aarch64"},
			}),
		},
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
contents:
  packages:
    - ca-certificates-bundle
    # Requested via a version constraint satisfied through a "provides" alias:
    # there is no package literally named "go", so this resolves to a concrete
    # versioned package (e.g. "go>1.25" -> "go-1.26").
    - go>1.25
  EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				// amd64, aarch64 (as arm64), and the "index" union.
				resource.TestCheckResourceAttr("data.apko_config.this", "resolved_packages.%", "3"),
				checkResolvedPackages("data.apko_config.this"),
			),
		}},
	})
}

// checkResolvedPackages validates the resolved_packages attribute:
//   - every per-arch entry has a name, an arch-appropriate .apk URL, and an
//     APKv2 ("Q1...") checksum;
//   - the "go>1.25" spec resolves through its "provides" alias to a concrete
//     versioned package (e.g. "go-1.26"); and
//   - the "index" list is the union of the per-arch lists.
func checkResolvedPackages(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource not found: %s", name)
		}
		att := rs.Primary.Attributes

		goRe := regexp.MustCompile(`^go-1\.\d+$`)
		apkv2Re := regexp.MustCompile(`^Q1[A-Za-z0-9+/=]+$`)
		urlRe := map[string]*regexp.Regexp{
			"amd64": regexp.MustCompile(`^https://packages\.wolfi\.dev/os/x86_64/.+\.apk$`),
			"arm64": regexp.MustCompile(`^https://packages\.wolfi\.dev/os/aarch64/.+\.apk$`),
		}

		counts := map[string]int{}
		for _, arch := range []string{"amd64", "arm64"} {
			n, err := strconv.Atoi(att[fmt.Sprintf("resolved_packages.%s.#", arch)])
			if err != nil {
				return fmt.Errorf("missing resolved_packages.%s.#: %w", arch, err)
			}
			if n == 0 {
				return fmt.Errorf("expected non-empty resolved_packages for %s", arch)
			}
			counts[arch] = n

			foundGo := false
			for i := 0; i < n; i++ {
				p := fmt.Sprintf("resolved_packages.%s.%d.", arch, i)
				if att[p+"name"] == "" {
					return fmt.Errorf("%sname is empty", p)
				}
				if goRe.MatchString(att[p+"name"]) {
					foundGo = true
				}
				if u := att[p+"url"]; !urlRe[arch].MatchString(u) {
					return fmt.Errorf("%surl = %q, want match %s", p, u, urlRe[arch])
				}
				if c := att[p+"apkv2"]; !apkv2Re.MatchString(c) {
					return fmt.Errorf("%sapkv2 = %q, want match %s", p, c, apkv2Re)
				}
			}
			if !foundGo {
				return fmt.Errorf("resolved_packages.%s did not contain a concrete go-1.x package resolved via the %q provides alias", arch, "go>1.25")
			}
		}

		idx, err := strconv.Atoi(att["resolved_packages.index.#"])
		if err != nil {
			return fmt.Errorf("missing resolved_packages.index.#: %w", err)
		}
		if want := counts["amd64"] + counts["arm64"]; idx != want {
			return fmt.Errorf("resolved_packages.index has %d entries, want union %d (amd64=%d + arm64=%d)", idx, want, counts["amd64"], counts["arm64"])
		}
		return nil
	}
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
