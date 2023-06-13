package provider

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	ocitesting "github.com/chainguard-dev/terraform-provider-oci/testing"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
data "apko_config" "foo" {
  config_contents = <<EOF
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

resource "apko_build" "foo" {
  repo   = %q
  config = data.apko_config.foo.config
}
`, repostr),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"apko_build.foo", "repo", regexp.MustCompile("^"+repostr)),
					resource.TestMatchResourceAttr(
						"apko_build.foo", "image_ref", regexp.MustCompile("^"+repostr+"@sha256:")),
					resource.TestCheckResourceAttr("apko_build.foo", "sboms.%", "3"),
				),
			},
			// Update the config and make sure the image gets rebuilt.
			{
				Config: fmt.Sprintf(`
data "apko_config" "foo" {
  config_contents = <<EOF
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
}

resource "apko_build" "foo" {
	repo   = %q
	config = data.apko_config.foo.config
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
				packages:     []string{"wolfi-baselayout"},
			}),
		}, Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "apko_config" "foo" {
  config_contents = <<EOF
contents:
  packages:
    - ca-certificates-bundle
    - tzdata
EOF
}

resource "apko_build" "foo" {
	repo   = %q
	config = data.apko_config.foo.config
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
data "apko_config" "foo" {
  config_contents = <<EOF
contents:
  packages:
    - ca-certificates-bundle
    - tzdata
    - busybox # <-- add busybox
EOF
}

resource "apko_build" "foo" {
	repo   = %q
	config = data.apko_config.foo.config
}
`, repostr),
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

func TestAccResourceApkoBuild_BuildDateEpoch(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	repostr := repo.String()

	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories: []string{"https://packages.wolfi.dev/os"},
				keyring:      []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:        []string{"x86_64"},
				packages:     []string{"wolfi-baselayout=20230201-r0"},
			}),
		},
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`
data "apko_config" "foo" {
  config_contents = <<EOF
contents:
  packages:
  - ca-certificates-bundle=20230506-r0
  - glibc-locale-posix=2.37-r6
  - tzdata=2023c-r0
EOF
}

resource "apko_build" "foo" {
  repo   = %q
  config = data.apko_config.foo.config
}
`, repostr),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("apko_build.foo", "repo", repostr),
				resource.TestCheckResourceAttr("apko_build.foo", "image_ref",
					// With pinned packages we should always get this digest.
					repo.Digest("sha256:334f171474bcc4fb81595489998c077d2916514d3296af8c9952a242f9a0d9d3").String()),
				resource.TestMatchResourceAttr("apko_build.foo", `sboms.amd64.predicate`,
					// With (these) pinned packages we should see the Unix
					// epoch because these packages weren't embedding
					// build date.
					regexp.MustCompile(regexp.QuoteMeta(fmt.Sprintf(`"created": %q`, time.Unix(0, 0).UTC().Format(time.RFC3339))))),
			),
		}},
	})

	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories: []string{"https://packages.wolfi.dev/os"},
				keyring:      []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:        []string{"x86_64"},
				packages:     []string{"wolfi-baselayout=20230201-r3"},
			}),
		},
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`
data "apko_config" "foo" {
  config_contents = <<EOF
contents:
  packages:
  - ca-certificates-bundle=20230506-r0
  - glibc-locale-posix=2.37-r6
  - tzdata=2023c-r0
EOF
}

resource "apko_build" "foo" {
  repo   = %q
  config = data.apko_config.foo.config
}
`, repostr),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("apko_build.foo", "repo", repostr),
				resource.TestCheckResourceAttr("apko_build.foo", "image_ref",
					// With pinned packages we should always get this digest.
					repo.Digest("sha256:592a4628161006424a3c6a2598b5e6b590eb96690a9e710cd156065e9316b81d").String()),
				resource.TestMatchResourceAttr("apko_build.foo", `sboms.amd64.predicate`,
					// With (these) pinned packages we should see the build date
					// from the APKINDEX for wolfi-baselayout which is 1686086025.
					regexp.MustCompile(regexp.QuoteMeta(fmt.Sprintf(`"created": %q`, time.Unix(1686086025, 0).UTC().Format(time.RFC3339))))),
			),
		}},
	})
}
