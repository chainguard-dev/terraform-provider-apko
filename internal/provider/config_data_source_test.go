package provider

import (
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestAccDataSourceConfig(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
  archs:
  - amd64
  - aarch64
  EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.#", "2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "x86_64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.1", "aarch64"),
			),
		}, {
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
  archs:
  - x86_64
  - arm64
  EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.#", "2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "x86_64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.1", "aarch64"),
			),
		}},
	})
}

func TestAccDataSourceConfig_ProviderOpts_Locked(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories: []string{"https://packages.wolfi.dev/os"},
				keyring:      []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:        []string{"x86_64", "aarch64"},
				packages:     []string{"wolfi-baselayout=20230201-r0"},
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
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "x86_64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.1", "aarch64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.#", "4"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.0", "ca-certificates-bundle=20230506-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.1", "glibc-locale-posix=2.37-r6"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.2", "tzdata=2023c-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.3", "wolfi-baselayout=20230201-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.repositories.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.repositories.0", "https://packages.wolfi.dev/os"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.keyring.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.keyring.0", "https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"),
				// Older Wolfi APKs don't specify build date.
				resource.TestCheckResourceAttr("data.apko_config.this", "apk_date_epoch", "1970-01-01T00:00:00Z"),
			),
		}},
	})
}

func TestAccDataSourceConfig_ProviderOpts_Unlocked(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories: []string{"https://packages.wolfi.dev/os"},
				keyring:      []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:        []string{"x86_64", "aarch64"},
				packages:     []string{"wolfi-baselayout"},
			}),
		},
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
contents:
  packages:
    - ca-certificates-bundle
    - tzdata
  EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.#", "2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "x86_64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.1", "aarch64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.#", "4"),
				resource.TestMatchResourceAttr("data.apko_config.this", "config.contents.packages.0", regexp.MustCompile("^ca-certificates-bundle=.*")),
				// This is pulled in as a transitive dependency.
				resource.TestMatchResourceAttr("data.apko_config.this", "config.contents.packages.1", regexp.MustCompile("^glibc-locale-posix=.*")),
				resource.TestMatchResourceAttr("data.apko_config.this", "config.contents.packages.2", regexp.MustCompile("^tzdata=.*")),
				resource.TestMatchResourceAttr("data.apko_config.this", "config.contents.packages.3", regexp.MustCompile("^wolfi-baselayout=.*")),
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
				repositories: []string{"https://packages.wolfi.dev/os"},
				keyring:      []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:        []string{"x86_64", "aarch64"},
				packages:     []string{"wolfi-baselayout"},
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
    - ca-certificates-bundle
    - tzdata
  EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "aarch64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.#", "4"),
				resource.TestMatchResourceAttr("data.apko_config.this", "config.contents.packages.0", regexp.MustCompile("^ca-certificates-bundle=.*")),
				// This is pulled in as a transitive dependency.
				resource.TestMatchResourceAttr("data.apko_config.this", "config.contents.packages.1", regexp.MustCompile("^glibc-locale-posix=.*")),
				resource.TestMatchResourceAttr("data.apko_config.this", "config.contents.packages.2", regexp.MustCompile("^tzdata=.*")),
				resource.TestMatchResourceAttr("data.apko_config.this", "config.contents.packages.3", regexp.MustCompile("^wolfi-baselayout=.*")),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.repositories.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.repositories.0", "https://packages.wolfi.dev/os"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.keyring.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.keyring.0", "https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"),
			),
		}},
	})
}

func TestAccDataSourceConfig_Alpine_Locked(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories: []string{"https://dl-cdn.alpinelinux.org/alpine/edge/main"},
				archs:        []string{"x86_64", "aarch64"},
			}),
		},
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
contents:
  packages:
    - ca-certificates-bundle=20230506-r0
    - tzdata=2023c-r1
  EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.#", "2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "x86_64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.1", "aarch64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.#", "2"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.0", "ca-certificates-bundle=20230506-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.1", "tzdata=2023c-r1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.repositories.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.repositories.0", "https://dl-cdn.alpinelinux.org/alpine/edge/main"),
				// We have locked a set of alpine packages, and this is the max build date.
				resource.TestCheckResourceAttr("data.apko_config.this", "apk_date_epoch", "2023-05-06T12:08:21Z"),
			),
		}},
	})
}

func TestUnify(t *testing.T) {
	tests := []struct {
		name      string
		originals []string
		inputs    []resolved
		want      []string
		wantDiag  diag.Diagnostics
	}{{
		name: "empty",
	}, {
		name:      "simple single arch",
		originals: []string{"foo", "bar", "baz"},
		inputs: []resolved{{
			packages: sets.New("foo", "bar", "baz"),
			versions: map[string]string{
				"foo": "1.2.3",
				"bar": "2.4.6",
				"baz": "0.0.1",
			},
		}},
		want: []string{
			"bar=2.4.6",
			"baz=0.0.1",
			"foo=1.2.3",
		},
	}, {
		name:      "locked versions",
		originals: []string{"foo=1.2.3", "bar=2.4.6", "baz=0.0.1"},
		inputs: []resolved{{
			packages: sets.New("foo", "bar", "baz"),
			versions: map[string]string{
				"foo": "1.2.3",
				"bar": "2.4.6",
				"baz": "0.0.1",
			},
		}},
		want: []string{
			"bar=2.4.6",
			"baz=0.0.1",
			"foo=1.2.3",
		},
	}, {
		name:      "transitive dependency",
		originals: []string{"foo", "bar", "baz"},
		inputs: []resolved{{
			packages: sets.New("foo", "bar", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"bar":   "2.4.6",
				"baz":   "0.0.1",
				"bonus": "5.4.3",
			},
		}},
		want: []string{
			"bar=2.4.6",
			"baz=0.0.1",
			"bonus=5.4.3",
			"foo=1.2.3",
		},
	}, {
		name:      "multiple matching architectures",
		originals: []string{"foo", "bar", "baz"},
		inputs: []resolved{{
			arch:     "x86_64",
			packages: sets.New("foo", "bar", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"bar":   "2.4.6",
				"baz":   "0.0.1",
				"bonus": "5.4.3",
			},
		}, {
			arch:     "aarch64",
			packages: sets.New("foo", "bar", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"bar":   "2.4.6",
				"baz":   "0.0.1",
				"bonus": "5.4.3",
			},
		}},
		want: []string{
			"bar=2.4.6",
			"baz=0.0.1",
			"bonus=5.4.3",
			"foo=1.2.3",
		},
	}, {
		name:      "mismatched transitive dependency",
		originals: []string{"foo", "bar", "baz"},
		inputs: []resolved{{
			arch:     "x86_64",
			packages: sets.New("foo", "bar", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"bar":   "2.4.6",
				"baz":   "0.0.1",
				"bonus": "5.4.3-r0",
			},
		}, {
			arch:     "aarch64",
			packages: sets.New("foo", "bar", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"bar":   "2.4.6",
				"baz":   "0.0.1",
				"bonus": "5.4.3-r1",
			},
		}},
		want: []string{
			"bar=2.4.6",
			"baz=0.0.1",
			"foo=1.2.3",
		},
		wantDiag: []diag.Diagnostic{
			diag.NewWarningDiagnostic("unable to lock certain packages for x86_64", "[bonus]"),
			diag.NewWarningDiagnostic("unable to lock certain packages for aarch64", "[bonus]"),
		},
	}, {
		name:      "mismatched direct dependency",
		originals: []string{"foo", "bar", "baz"},
		inputs: []resolved{{
			arch:     "x86_64",
			packages: sets.New("foo", "bar", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"bar":   "2.4.6-r0",
				"baz":   "0.0.1",
				"bonus": "5.4.3",
			},
		}, {
			arch:     "aarch64",
			packages: sets.New("foo", "bar", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"bar":   "2.4.6-r1",
				"baz":   "0.0.1",
				"bonus": "5.4.3",
			},
		}},
		want: []string{
			"bar",
			"baz=0.0.1",
			"bonus=5.4.3",
			"foo=1.2.3",
		},
		wantDiag: []diag.Diagnostic{
			diag.NewWarningDiagnostic("unable to lock certain packages", "[bar]"),
		},
	}, {
		name:      "mismatched direct dependency (with constraint)",
		originals: []string{"foo", "bar>2.4.6", "baz"},
		inputs: []resolved{{
			arch:     "x86_64",
			packages: sets.New("foo", "bar", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"bar":   "2.4.6-r0",
				"baz":   "0.0.1",
				"bonus": "5.4.3",
			},
		}, {
			arch:     "aarch64",
			packages: sets.New("foo", "bar", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"bar":   "2.4.6-r1",
				"baz":   "0.0.1",
				"bonus": "5.4.3",
			},
		}},
		want: []string{
			"bar>2.4.6", // Check that we keep our input constraint
			"baz=0.0.1",
			"bonus=5.4.3",
			"foo=1.2.3",
		},
		wantDiag: []diag.Diagnostic{
			diag.NewWarningDiagnostic("unable to lock certain packages", "[bar]"),
		},
	}, {
		name:      "single-architecture resolved dependency",
		originals: []string{"foo", "bar", "baz"},
		inputs: []resolved{{
			arch:     "x86_64",
			packages: sets.New("foo", "bar", "baz", "intel-fast-as-f-math"),
			versions: map[string]string{
				"foo":                  "1.2.3",
				"bar":                  "2.4.6",
				"baz":                  "0.0.1",
				"intel-fast-as-f-math": "5.4.3",
			},
		}, {
			arch:     "aarch64",
			packages: sets.New("foo", "bar", "baz", "arm-energy-efficient-as-f-arithmetic"),
			versions: map[string]string{
				"foo":                                  "1.2.3",
				"bar":                                  "2.4.6",
				"baz":                                  "0.0.1",
				"arm-energy-efficient-as-f-arithmetic": "9.8.7",
			},
		}},
		want: []string{
			"bar=2.4.6",
			"baz=0.0.1",
			"foo=1.2.3",
		},
		wantDiag: []diag.Diagnostic{
			diag.NewWarningDiagnostic("unable to lock certain packages for x86_64", "[intel-fast-as-f-math]"),
			diag.NewWarningDiagnostic("unable to lock certain packages for aarch64", "[arm-energy-efficient-as-f-arithmetic]"),
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _, gotDiag := unify(test.originals, test.inputs)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("(-want, +got) = %s", diff)
			}
			if diff := cmp.Diff(test.wantDiag, gotDiag); diff != "" {
				t.Errorf("(-want, +got) = %s", diff)
			}
		})
	}
}
