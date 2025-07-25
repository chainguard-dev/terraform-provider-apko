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
	"github.com/google/go-containerregistry/pkg/crane"
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
				repositories:       []string{"https://packages.wolfi.dev/os"},
				buildRespositories: []string{"./packages"},
				keyring:            []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:              []string{"x86_64", "aarch64"},
				packages:           []string{"wolfi-baselayout"},
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
				repositories:       []string{"https://packages.wolfi.dev/os"},
				buildRespositories: []string{"./packages"},
				keyring:            []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:              []string{"x86_64"},
				packages:           []string{"wolfi-baselayout=20230201-r0"},
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
					repo.Digest("sha256:4f2f87f6a611d89c6b47d87827f22efc4376baf06f85ae6d06c337fafb4089c2").String()),

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
				repositories:       []string{"https://packages.wolfi.dev/os"},
				buildRespositories: []string{"./packages"},
				keyring:            []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:              []string{"x86_64"},
				packages:           []string{"wolfi-baselayout=20230201-r3"},
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
					repo.Digest("sha256:91bee4d74207f37d2d2b15c7c54bf5367fcfdef33b888bb7ae88893643ef933e").String()),

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

func TestAccResourceApkoBuild_OldPackages(t *testing.T) {
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
		}, Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "apko_config" "foo" {
  config_contents = <<EOF
contents:
  packages:
    # Testing that old packages with potentially invalid SBOMs can produce a valid image SBOM.
    - glibc=2.36-r3
    - binutils=2.39-r4
    - git=2.39.0-r0
    - openssl=3.0.7-r0
    - sysstat=12.6.2-r0
    - libcrypto3=3.0.8-r0
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

// Test that this things builds if you give it "configs".
func TestAccResourceApkoBuild_PerArchConfigs(t *testing.T) {
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
		}, Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "apko_config" "foo" {
  config_contents = <<EOF
contents:
  packages:
    # This package pulls in libpciaccess only for x86_64.
    - libdrm=2.4.124-r0
  EOF
}

resource "apko_build" "foo" {
	repo    = %q
	config  = data.apko_config.foo.config
	configs = data.apko_config.foo.configs
}
`, repostr),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"apko_build.foo", "repo", regexp.MustCompile("^"+repostr)),
					resource.TestMatchResourceAttr(
						"apko_build.foo", "image_ref", regexp.MustCompile("^"+repostr+"@sha256:")),
					resource.TestCheckResourceAttr("data.apko_config.foo", "configs.%", "3"),
				),
			},
		},
	})
}

// Use layers!
func TestAccResourceApkoBuild_Layers(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	repostr := repo.String()

	// Need to update this if apko changes.
	digest := repo.Digest("sha256:022e1d4400460b17ea397e73b175d6d37a780c3d945ded4cd996692adc77e229")

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
		}, Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "apko_config" "foo" {
  config_contents = <<EOF
contents:
  packages:
    - ca-certificates-bundle=20230506-r0
    - glibc-locale-posix=2.37-r6
    - tzdata=2023c-r0
layering:
  strategy: origin
  budget: 10
  EOF
}

resource "apko_build" "foo" {
	repo    = %q
	config  = data.apko_config.foo.config
	configs = data.apko_config.foo.configs
}
`, repostr),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"apko_build.foo", "repo", regexp.MustCompile("^"+repostr)),
					// With pinned packages we should always get this digest.
					resource.TestCheckResourceAttr("apko_build.foo", "image_ref", digest.String()),
				),
			},
		},
	})

	img, err := crane.Pull(digest.String())
	if err != nil {
		t.Fatal(err)
	}
	m, err := img.Manifest()
	if err != nil {
		t.Fatal(err)
	}

	// 5 = len(packages) + wolfi-baselayout + top layer
	if got, want := len(m.Layers), 5; got != want {
		t.Errorf("len(layers): %d, want %d", got, want)
	}
}
