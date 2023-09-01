package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"chainguard.dev/apko/pkg/sbom/generator/spdx"
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
					repo.Digest("sha256:14f90e68f2603e992cc51916cc70113b1037bc2c5c6566ca26fec292753f3360").String()),

				// Check that the build's amd64 predicate exists, the digest
				// matches, and the creation timestamp is what we expect.
				resource.TestCheckFunc(func(s *terraform.State) error {
					ms := s.RootModule()
					rs, ok := ms.Resources["apko_build.foo"]
					if !ok {
						return errors.New("unable to find build resource foo")
					}
					path := rs.Primary.Attributes["sboms.amd64.predicate_path"]
					sbom, err := os.ReadFile(path)
					if err != nil {
						return fmt.Errorf("reading sboms.amd64.predicate_path: %w", err)
					}

					// Check that the hash matches.
					rawHash := sha256.Sum256(sbom)
					if got, want := hex.EncodeToString(rawHash[:]), rs.Primary.Attributes["sboms.amd64.predicate_sha256"]; got != want {
						return fmt.Errorf("got sha256 %q, wanted %q", got, want)
					}

					// With (these) pinned packages we should see the UTC Unix
					// epoch because these packages weren't embedding
					// build date.
					var doc spdx.Document
					if err := json.Unmarshal(sbom, &doc); err != nil {
						return err
					}
					if got, want := doc.CreationInfo.Created, time.Unix(0, 0).UTC().Format(time.RFC3339); got != want {
						return fmt.Errorf("got created %s, wanted %s", got, want)
					}
					return nil
				}),
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
					repo.Digest("sha256:7ecfd5ca0cec575e96ff46bf53284950ee80e5d1211cba47a51189debe41c79a").String()),

				// Check that the build's amd64 predicate exists, the digest
				// matches, and the creation timestamp is what we expect.
				resource.TestCheckFunc(func(s *terraform.State) error {
					ms := s.RootModule()
					rs, ok := ms.Resources["apko_build.foo"]
					if !ok {
						return errors.New("unable to find build resource foo")
					}
					path := rs.Primary.Attributes["sboms.amd64.predicate_path"]
					sbom, err := os.ReadFile(path)
					if err != nil {
						return err
					}

					// Check that the hash matches.
					rawHash := sha256.Sum256(sbom)
					if got, want := hex.EncodeToString(rawHash[:]), rs.Primary.Attributes["sboms.amd64.predicate_sha256"]; got != want {
						return fmt.Errorf("got sha256 %q, wanted %q", got, want)
					}

					// With (these) pinned packages we should see the build date
					// from the APKINDEX for wolfi-baselayout which is 1686086025.
					var doc spdx.Document
					if err := json.Unmarshal(sbom, &doc); err != nil {
						return err
					}
					if got, want := doc.CreationInfo.Created, time.Unix(1686086025, 0).UTC().Format(time.RFC3339); got != want {
						return fmt.Errorf("got created %s, wanted %s", got, want)
					}
					return nil
				}),
			),
		}},
	})
}
