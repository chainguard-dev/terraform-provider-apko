package provider

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TODO: move this into a shared testing library where tf-{ko,apko,cosign} can use it too.
func setupRegistry(t *testing.T) (name.Registry, func()) {
	t.Helper()
	if got := os.Getenv("TF_OCI_REGISTRY"); got != "" {
		reg, err := name.NewRegistry(got)
		if err != nil {
			t.Fatalf("failed to parse TF_OCI_REGISTRY: %v", err)
		}
		return reg, func() {}
	}
	srv := httptest.NewServer(registry.New())
	t.Logf("Started registry: %s", srv.URL)
	reg, err := name.NewRegistry(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("failed to parse TF_OCI_REGISTRY: %v", err)
	}
	return reg, srv.Close
}

func TestAccResourceApkoBuild(t *testing.T) {
	reg, cleanup := setupRegistry(t)
	defer cleanup()

	repo, err := name.NewRepository(path.Join(reg.RegistryStr(), "test"))
	if err != nil {
		t.Fatal(err)
	}
	repostr := repo.String()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
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
}`, repostr),
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr(
					"apko_build.foo", "repo", regexp.MustCompile("^"+repostr)),
				resource.TestMatchResourceAttr(
					"apko_build.foo", "image_ref", regexp.MustCompile("^"+repostr+"@sha256:")),
			),
		}},
	})
}
