package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"chainguard.dev/apko/pkg/build/types"
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
				packages:           []string{"wolfi-baselayout=20230201-r0"},
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
  - ca-certificates-bundle=20230506-r0
  - glibc-locale-posix=2.37-r6
annotations:
  bar: config-provided
EOF
  extra_packages = ["tzdata=2023c-r0"]
  default_annotations = {
	foo: "bar"
	bar: "datasource-provided"
  }
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.#", "4"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.0", "ca-certificates-bundle=20230506-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.1", "glibc-locale-posix=2.37-r6"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.2", "tzdata=2023c-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.3", "wolfi-baselayout=20230201-r0"),
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
				packages:           []string{"wolfi-baselayout=20230201-r0"},
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
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.0", "amd64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.archs.1", "arm64"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.#", "4"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.0", "ca-certificates-bundle=20230506-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.1", "glibc-locale-posix=2.37-r6"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.2", "tzdata=2023c-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.3", "wolfi-baselayout=20230201-r0"),
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
			arch:     "amd64",
			packages: sets.New("foo", "bar", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"bar":   "2.4.6",
				"baz":   "0.0.1",
				"bonus": "5.4.3",
			},
			provided: map[string]sets.Set[string]{
				"foo": sets.New("abc", "ogg"),
				"bar": sets.New("def"),
			},
		}, {
			arch:     "arm64",
			packages: sets.New("foo", "bar", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"bar":   "2.4.6",
				"baz":   "0.0.1",
				"bonus": "5.4.3",
			},
			provided: map[string]sets.Set[string]{
				"foo": sets.New("abc"),
				"bar": sets.New("def", "ogg"),
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
			arch:     "amd64",
			packages: sets.New("foo", "bar", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"bar":   "2.4.6",
				"baz":   "0.0.1",
				"bonus": "5.4.3-r0",
			},
		}, {
			arch:     "arm64",
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
			diag.NewWarningDiagnostic("unable to lock certain packages for amd64", "[bonus]"),
			diag.NewWarningDiagnostic("unable to lock certain packages for arm64", "[bonus]"),
		},
	}, {
		name:      "provided direct dependency",
		originals: []string{"foo", "bar", "baz"},
		inputs: []resolved{{
			arch:     "amd64",
			packages: sets.New("foo", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"baz":   "0.0.1",
				"bonus": "5.4.3",
			},
			provided: map[string]sets.Set[string]{
				"bonus": sets.New("bar"),
			},
		}, {
			arch:     "arm64",
			packages: sets.New("foo", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"baz":   "0.0.1",
				"bonus": "5.4.3",
			},
			provided: map[string]sets.Set[string]{
				"bonus": sets.New("bar"),
			},
		}},
		want: []string{
			"baz=0.0.1",
			"bonus=5.4.3",
			"foo=1.2.3",
		},
	}, {
		name:      "mismatched direct dependency",
		originals: []string{"foo", "bar", "baz"},
		inputs: []resolved{{
			arch:     "amd64",
			packages: sets.New("foo", "bar", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"bar":   "2.4.6-r0",
				"baz":   "0.0.1",
				"bonus": "5.4.3",
			},
		}, {
			arch:     "arm64",
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
			diag.NewErrorDiagnostic(
				`Unable to lock package "bar" to a consistent version`,
				"2.4.6-r0 (amd64), 2.4.6-r1 (arm64)",
			),
		},
	}, {
		name:      "mismatched direct dependency (with constraint)",
		originals: []string{"foo", "bar>2.4.6", "baz"},
		inputs: []resolved{{
			arch:     "amd64",
			packages: sets.New("foo", "bar", "baz", "bonus"),
			versions: map[string]string{
				"foo":   "1.2.3",
				"bar":   "2.4.6-r0",
				"baz":   "0.0.1",
				"bonus": "5.4.3",
			},
		}, {
			arch:     "arm64",
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
			diag.NewErrorDiagnostic(
				`Unable to lock package "bar" to a consistent version`,
				"2.4.6-r0 (amd64), 2.4.6-r1 (arm64)",
			),
		},
	}, {
		name:      "single-architecture resolved dependency",
		originals: []string{"foo", "bar", "baz"},
		inputs: []resolved{{
			arch:     "amd64",
			packages: sets.New("foo", "bar", "baz", "intel-fast-as-f-math"),
			versions: map[string]string{
				"foo":                  "1.2.3",
				"bar":                  "2.4.6",
				"baz":                  "0.0.1",
				"intel-fast-as-f-math": "5.4.3",
			},
		}, {
			arch:     "arm64",
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
			diag.NewWarningDiagnostic("unable to lock certain packages for amd64", "[intel-fast-as-f-math]"),
			diag.NewWarningDiagnostic("unable to lock certain packages for arm64", "[arm-energy-efficient-as-f-arithmetic]"),
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, gotDiag := unify(test.originals, test.inputs)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("(-want, +got) = %s", diff)
			}
			if diff := cmp.Diff(test.wantDiag, gotDiag); diff != "" {
				t.Errorf("(-want, +got) = %s", diff)
			}
		})
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

func TestAccDataSourceConfig_RemoteBuilder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(types.ImageConfiguration{
			Contents: types.ImageContents{
				Packages: []string{
					"ca-certificates-bundle=20230506-r0",
					"glibc-locale-posix=2.37-r6",
					"tzdata=2023c-r0",
					"wolfi-baselayout=20230201-r0",
				},
			},
		})
	}))
	defer srv.Close()

	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				remoteBuilder: &srv.URL,
			}),
		},
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
contents:
  packages:
  - does-not-matter
EOF
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.#", "4"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.0", "ca-certificates-bundle=20230506-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.1", "glibc-locale-posix=2.37-r6"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.2", "tzdata=2023c-r0"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.3", "wolfi-baselayout=20230201-r0"),
			),
		}},
	})
}
