package provider

import (
	"context"
	"testing"

	"chainguard.dev/apko/pkg/apk/apk"
	apkotypes "chainguard.dev/apko/pkg/build/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// resolvedPkgModel mirrors resolvedPackageSchema so we can read the produced
// Terraform value back into plain Go and compare it with cmp.Diff.
type resolvedPkgModel struct {
	Name             string   `tfsdk:"name"`
	Version          string   `tfsdk:"version"`
	Arch             string   `tfsdk:"arch"`
	URL              string   `tfsdk:"url"`
	Checksum         string   `tfsdk:"checksum"`
	Origin           string   `tfsdk:"origin"`
	Description      string   `tfsdk:"description"`
	License          string   `tfsdk:"license"`
	Maintainer       string   `tfsdk:"maintainer"`
	Homepage         string   `tfsdk:"homepage"`
	RepoCommit       string   `tfsdk:"repo_commit"`
	BuildDate        int64    `tfsdk:"build_date"`
	Size             int64    `tfsdk:"size"`
	InstalledSize    int64    `tfsdk:"installed_size"`
	ProviderPriority int64    `tfsdk:"provider_priority"`
	Provides         []string `tfsdk:"provides"`
	Dependencies     []string `tfsdk:"dependencies"`
}

// mkResolvedPkg builds a RepositoryPackage from static APKINDEX-style metadata.
// The repository URI (and therefore rp.URL()) is derived from the components,
// standing in for a static index rather than a live one.
func mkResolvedPkg(arch string, p apk.Package) *apk.RepositoryPackage {
	repo := apk.NewRepositoryFromComponents("https://example.test", "os", "main", arch)
	rwi := repo.WithIndex(&apk.APKIndex{})
	return apk.NewRepositoryPackage(&p, rwi)
}

func TestResolvedPackagesMap(t *testing.T) {
	goAmd := apk.Package{
		Name: "go-1.26", Version: "1.26.5-r1", Arch: "x86_64",
		Description: "the go programming language", License: "BSD-3-Clause",
		Origin: "go-1.26", Maintainer: "Wolfi <maint@example.test>",
		URL:          "https://go.dev/",
		Checksum:     []byte{1, 2, 3, 4},
		Dependencies: []string{"so:libc.so.6"},
		Provides:     []string{"go=1.26.5-r1", "cmd:go=1.26.5-r1"},
		Size:         100, InstalledSize: 200, ProviderPriority: 5,
		BuildDate: 1700000000, RepoCommit: "abc123",
	}
	caAmd := apk.Package{
		Name: "ca-certificates-bundle", Version: "20260413-r0", Arch: "x86_64",
		Description: "CA bundle", License: "MPL-2.0", Origin: "ca-certificates",
		Maintainer: "Wolfi <maint@example.test>",
		URL:        "https://example.test/ca",
		Checksum:   []byte{5, 6, 7, 8},
		Size:       10, InstalledSize: 20,
		BuildDate: 1700000001, RepoCommit: "def456",
	}
	goArm := goAmd
	goArm.Arch = "aarch64"
	goArm.Checksum = []byte{9, 10, 11, 12}

	resolved := map[apkotypes.Architecture][]*apk.RepositoryPackage{
		apkotypes.Architecture("amd64"): {mkResolvedPkg("x86_64", goAmd), mkResolvedPkg("x86_64", caAmd)},
		apkotypes.Architecture("arm64"): {mkResolvedPkg("aarch64", goArm)},
	}

	mapVal, diags := resolvedPackagesMap(resolved)
	if diags.HasError() {
		t.Fatalf("resolvedPackagesMap: %v", diags)
	}

	var got map[string][]resolvedPkgModel
	if d := mapVal.ElementsAs(context.Background(), &got, false); d.HasError() {
		t.Fatalf("ElementsAs: %v", d)
	}

	goAmdModel := resolvedPkgModel{
		Name: "go-1.26", Version: "1.26.5-r1", Arch: "x86_64",
		URL:         "https://example.test/os/main/x86_64/go-1.26-1.26.5-r1.apk",
		Checksum:    "Q1AQIDBA==",
		Origin:      "go-1.26",
		Description: "the go programming language", License: "BSD-3-Clause",
		Maintainer: "Wolfi <maint@example.test>", Homepage: "https://go.dev/",
		RepoCommit: "abc123", BuildDate: 1700000000,
		Size: 100, InstalledSize: 200, ProviderPriority: 5,
		Provides:     []string{"go=1.26.5-r1", "cmd:go=1.26.5-r1"},
		Dependencies: []string{"so:libc.so.6"},
	}
	caAmdModel := resolvedPkgModel{
		Name: "ca-certificates-bundle", Version: "20260413-r0", Arch: "x86_64",
		URL:         "https://example.test/os/main/x86_64/ca-certificates-bundle-20260413-r0.apk",
		Checksum:    "Q1BQYHCA==",
		Origin:      "ca-certificates",
		Description: "CA bundle", License: "MPL-2.0",
		Maintainer: "Wolfi <maint@example.test>", Homepage: "https://example.test/ca",
		RepoCommit: "def456", BuildDate: 1700000001,
		Size: 10, InstalledSize: 20,
	}
	goArmModel := goAmdModel
	goArmModel.Arch = "aarch64"
	goArmModel.URL = "https://example.test/os/main/aarch64/go-1.26-1.26.5-r1.apk"
	goArmModel.Checksum = "Q1CQoLDA=="

	want := map[string][]resolvedPkgModel{
		"amd64": {goAmdModel, caAmdModel},
		"arm64": {goArmModel},
		// index is the deduplicated union across architectures, in sorted-arch
		// order (amd64 then arm64).
		"index": {goAmdModel, caAmdModel, goArmModel},
	}

	if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("resolvedPackagesMap mismatch (-want +got):\n%s", diff)
	}
}
