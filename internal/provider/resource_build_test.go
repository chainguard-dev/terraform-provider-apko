package provider

import (
	"fmt"
	"regexp"
	"testing"

	ocitesting "github.com/chainguard-dev/terraform-provider-oci/testing"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccResourceApkoBuild(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	repostr := repo.String()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
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
}

output "bar" {
  value = apko_build.foo.sboms
}
`, repostr),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"apko_build.foo", "repo", regexp.MustCompile("^"+repostr)),
					resource.TestMatchResourceAttr(
						"apko_build.foo", "image_ref", regexp.MustCompile("^"+repostr+"@sha256:")),
					func(s *terraform.State) error {
						ms := s.RootModule()
						rs, ok := ms.Outputs["bar"]
						if !ok {
							return fmt.Errorf("Not found: %s", "bar")
						}
						sboms, ok := rs.Value.(map[string]interface{})
						if !ok {
							return fmt.Errorf("Incorrect type %T", rs.Value)
						}
						if len(sboms) != 3 {
							return fmt.Errorf("wanted 3 SBOMs, got %d", len(sboms))
						}
						return nil
					},
				),
			},
			// Update the config and make sure the image gets rebuilt.
			{
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
    - git # <-- add git

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
}`, repostr),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"apko_build.foo", "repo", regexp.MustCompile("^"+repostr)),
					resource.TestMatchResourceAttr(
						"apko_build.foo", "image_ref", regexp.MustCompile("^"+repostr+"@sha256:")),
				),
			},
		},
	})
}

func TestAccResourceApkoBuild_ProviderOpts(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	repostr := repo.String()

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories: []string{"https://packages.wolfi.dev/os"},
				keyring:      []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:        []string{"x86_64", "aarch64"},
			}),
		}, Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "apko_build" "foo" {
  repo   = %q
  config = <<EOF
contents:
  packages:
    - wolfi-baselayout
    - ca-certificates-bundle
    - tzdata
EOF
}
`, repostr),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"apko_build.foo", "repo", regexp.MustCompile("^"+repostr)),
					resource.TestMatchResourceAttr(
						"apko_build.foo", "image_ref", regexp.MustCompile("^"+repostr+"@sha256:")),
				),
			},
			// Update the config and make sure the image gets rebuilt.
			{
				Config: fmt.Sprintf(`
resource "apko_build" "foo" {
  repo   = %q
  config = <<EOF
contents:
  packages:
    - wolfi-baselayout
    - ca-certificates-bundle
    - tzdata
    - git # <-- add git
EOF
}`, repostr),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"apko_build.foo", "repo", regexp.MustCompile("^"+repostr)),
					resource.TestMatchResourceAttr(
						"apko_build.foo", "image_ref", regexp.MustCompile("^"+repostr+"@sha256:")),
				),
			},
		},
	})
}
