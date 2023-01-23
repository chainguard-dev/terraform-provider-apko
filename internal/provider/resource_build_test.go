package provider

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccResourceApkoBuild(t *testing.T) {
	repo := os.Getenv("TEST_REPOSITORY")

	resource.UnitTest(t, resource.TestCase{
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`
resource "apko_build" "foo" {
  repo   = %q
  config = <<EOF
contents:
  repositories:
    - https://packages.wolfi.dev/os
  keyring:
    - https://packages.wolfi.dev/os/wolfi-signing.rsa.pub
  packages:
    - wolfi-baselayout
    - ca-certificates-bundle
    - tzdata

accounts:
  groups:
    - groupname: nonroot
      gid: 65532
  users:
    - username: nonroot
      uid: 65532
      gid: 65532
  run-as: 65532

archs:
  - x86_64
  - aarch64
EOF
}`, repo),
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr(
					"apko_build.foo", "repo", regexp.MustCompile("^"+repo)),
				resource.TestMatchResourceAttr(
					"apko_build.foo", "image_ref", regexp.MustCompile("^"+repo+"@sha256:")),
			),
		}},
	})
}
