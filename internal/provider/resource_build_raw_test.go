package provider

import (
	"fmt"
	"regexp"
	"testing"

	ocitesting "github.com/chainguard-dev/terraform-provider-oci/testing"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccResourceApkoBuildRaw tests a basic raw build where optional fields
// are omitted.
func TestAccResourceApkoBuildRaw(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	repostr := repo.String()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
locals {
  config = jsonencode({
    contents = {
      repositories = ["https://packages.wolfi.dev/os"]
      keyring      = ["https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"]
      packages     = ["wolfi-baselayout", "ca-certificates-bundle", "tzdata"]
    }
    archs = ["x86_64", "aarch64"]
  })
}

resource "apko_build_raw" "foo" {
  repo = %q
  # Mix canonical OCI ("arm64") and APK ("x86_64") arch names
  # to verify normalization handles both forms.
  configs_raw = {
    "index"  = local.config
    "x86_64" = local.config
    "arm64"  = local.config
  }
}
`, repostr),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"apko_build_raw.foo", "image_ref", regexp.MustCompile("^"+repostr+"@sha256:")),
					resource.TestCheckResourceAttr("apko_build_raw.foo", "sboms.%", "3"),
				),
			},
		},
	})
}

// TestAccResourceApkoBuildRaw_ProviderOpts tests that provider-level values
// merge correctly with raw configs.
func TestAccResourceApkoBuildRaw_ProviderOpts(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	repostr := repo.String()

	resource.Test(t, resource.TestCase{
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
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
locals {
  config = jsonencode({
    contents = {
      packages = ["ca-certificates-bundle", "tzdata"]
    }
  })
}

resource "apko_build_raw" "foo" {
  repo = %q
  configs_raw = {
    "index"   = local.config
    "amd64"   = local.config
    "aarch64" = local.config
  }
}
`, repostr),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"apko_build_raw.foo", "image_ref", regexp.MustCompile("^"+repostr+"@sha256:")),
				),
			},
		},
	})
}

// TestAccResourceApkoBuildRaw_PerArchConfigs tests per-architecture configs
// with different packages per arch.
func TestAccResourceApkoBuildRaw_PerArchConfigs(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	repostr := repo.String()

	resource.Test(t, resource.TestCase{
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
		Steps: []resource.TestStep{
			{
				// libdrm pulls in libpciaccess only on x86_64, so per-arch
				// configs let each arch carry its own resolved package set.
				Config: fmt.Sprintf(`
locals {
  index_config = jsonencode({
    contents = {
      packages = ["libdrm=2.4.131-r0"]
    }
    archs = ["x86_64", "aarch64"]
  })

  amd64_config = jsonencode({
    contents = {
      packages = ["libdrm=2.4.131-r0"]
    }
  })

  aarch64_config = jsonencode({
    contents = {
      packages = ["libdrm=2.4.131-r0"]
    }
  })
}

resource "apko_build_raw" "foo" {
  repo = %q
  configs_raw = {
    "index"   = local.index_config
    "amd64"   = local.amd64_config
    "aarch64" = local.aarch64_config
  }
}
`, repostr),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"apko_build_raw.foo", "image_ref", regexp.MustCompile("^"+repostr+"@sha256:")),
					resource.TestCheckResourceAttr("apko_build_raw.foo", "sboms.%", "3"),
				),
			},
		},
	})
}

// TestAccResourceApkoBuildRaw_SingleArch tests a single-architecture build.
func TestAccResourceApkoBuildRaw_SingleArch(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	repostr := repo.String()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
locals {
  config = jsonencode({
    contents = {
      repositories = ["https://packages.wolfi.dev/os"]
      keyring      = ["https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"]
      packages     = ["wolfi-baselayout", "ca-certificates-bundle"]
    }
    archs = ["x86_64"]
  })
}

resource "apko_build_raw" "foo" {
  repo = %q
  configs_raw = {
    "index" = local.config
    "amd64" = local.config
  }
}
`, repostr),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"apko_build_raw.foo", "image_ref", regexp.MustCompile("^"+repostr+"@sha256:")),
					resource.TestCheckResourceAttr("apko_build_raw.foo", "sboms.%", "2"),
				),
			},
		},
	})
}

// TestAccResourceApkoBuildRaw_MissingIndex verifies a clear error when "index" is absent.
func TestAccResourceApkoBuildRaw_MissingIndex(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	repostr := repo.String()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
locals {
  config = jsonencode({
    contents = {
      repositories = ["https://packages.wolfi.dev/os"]
      keyring      = ["https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"]
      packages     = ["wolfi-baselayout"]
    }
    archs = ["x86_64"]
  })
}

resource "apko_build_raw" "foo" {
  repo = %q
  configs_raw = {
    "amd64" = local.config
  }
}
`, repostr),
				ExpectError: regexp.MustCompile(`missing index configuration`),
			},
		},
	})
}

// TestAccResourceApkoBuildRaw_InvalidJSON verifies a clear error for malformed input.
func TestAccResourceApkoBuildRaw_InvalidJSON(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	repostr := repo.String()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "apko_build_raw" "foo" {
  repo = %q
  configs_raw = {
    "index" = "not valid json{{"
    "amd64" = "not valid json{{"
  }
}
`, repostr),
				ExpectError: regexp.MustCompile(`decoding config for`),
			},
		},
	})
}

// TestAccResourceApkoBuildRaw_DuplicateArch verifies that providing duplicate
// arch's produces a clear error.
func TestAccResourceApkoBuildRaw_DuplicateArch(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	repostr := repo.String()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
locals {
  config = jsonencode({
    contents = {
      repositories = ["https://packages.wolfi.dev/os"]
      keyring      = ["https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"]
      packages     = ["wolfi-baselayout"]
    }
    archs = ["aarch64"]
  })
}

resource "apko_build_raw" "foo" {
  repo = %q
  configs_raw = {
    "index"   = local.config
    "arm64"   = local.config
    "aarch64" = local.config
  }
}
`, repostr),
				ExpectError: regexp.MustCompile(`duplicate arch key`),
			},
		},
	})
}
