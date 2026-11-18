package provider

import (
	"regexp"
	"testing"

	"chainguard.dev/apko/pkg/apk/apk"
	apkotypes "chainguard.dev/apko/pkg/build/types"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDataSourceTags(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories: []string{"https://packages.wolfi.dev/os"},
				keyring:      []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:        []string{"x86_64", "aarch64"},
				packages:     []string{"wolfi-baselayout=20230201-r24"},
			}),
		},
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
contents:
  packages:
  - ca-certificates-bundle=20250911-r0
  - glibc-locale-posix=2.42-r2
  - ko=0.19.1-r1
  - nodejs=24.18.0-r1 # Initial request will be satisfied by 'provides'
EOF
  extra_packages = ["tzdata=2025b-r2"]
}

data "apko_tags" "ca-certs" {
  config         = data.apko_config.this.config
  target_package = "ca-certificates-bundle"
}

data "apko_tags" "glibc" {
  config         = data.apko_config.this.config
  target_package = "glibc-locale-posix"
}

data "apko_tags" "wolfi-baselayout" {
  config         = data.apko_config.this.config
  target_package = "wolfi-baselayout"
}

data "apko_tags" "tzdata" {
  config         = data.apko_config.this.config
  target_package = "tzdata"
}

data "apko_tags" "ko" {
  config         = data.apko_config.this.config
  target_package = "ko"
}

data "apko_tags" "nodejs" {
  config         = data.apko_config.this.config
  target_package = "nodejs" # Tags can be inferred from 'provides'
}

data "apko_tags" "nodejs-24" {
  config         = data.apko_config.this.config
  target_package = "nodejs-24"
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_tags.glibc", "tags.#", "3"),
				resource.TestCheckResourceAttr("data.apko_tags.glibc", "tags.0", "2"),
				resource.TestCheckResourceAttr("data.apko_tags.glibc", "tags.1", "2.42"),
				resource.TestCheckResourceAttr("data.apko_tags.glibc", "tags.2", "2.42-r2"),
				resource.TestCheckResourceAttr("data.apko_tags.glibc", "id", "2,2.42,2.42-r2"),

				resource.TestCheckResourceAttr("data.apko_tags.ca-certs", "tags.#", "2"),
				resource.TestCheckResourceAttr("data.apko_tags.ca-certs", "tags.0", "20250911"),
				resource.TestCheckResourceAttr("data.apko_tags.ca-certs", "tags.1", "20250911-r0"),
				resource.TestCheckResourceAttr("data.apko_tags.ca-certs", "id", "20250911,20250911-r0"),

				resource.TestCheckResourceAttr("data.apko_tags.wolfi-baselayout", "tags.#", "2"),
				resource.TestCheckResourceAttr("data.apko_tags.wolfi-baselayout", "tags.0", "20230201"),
				resource.TestCheckResourceAttr("data.apko_tags.wolfi-baselayout", "tags.1", "20230201-r24"),
				resource.TestCheckResourceAttr("data.apko_tags.wolfi-baselayout", "id", "20230201,20230201-r24"),

				resource.TestCheckResourceAttr("data.apko_tags.tzdata", "tags.#", "2"),
				resource.TestCheckResourceAttr("data.apko_tags.tzdata", "tags.0", "2025b"),
				resource.TestCheckResourceAttr("data.apko_tags.tzdata", "tags.1", "2025b-r2"),
				resource.TestCheckResourceAttr("data.apko_tags.tzdata", "id", "2025b,2025b-r2"),

				resource.TestCheckResourceAttr("data.apko_tags.ko", "tags.#", "4"),
				resource.TestCheckResourceAttr("data.apko_tags.ko", "tags.0", "0"),
				resource.TestCheckResourceAttr("data.apko_tags.ko", "tags.1", "0.19"),
				resource.TestCheckResourceAttr("data.apko_tags.ko", "tags.2", "0.19.1"),
				resource.TestCheckResourceAttr("data.apko_tags.ko", "tags.3", "0.19.1-r1"),
				resource.TestCheckResourceAttr("data.apko_tags.ko", "id", "0,0.19,0.19.1,0.19.1-r1"),

				resource.TestCheckResourceAttr("data.apko_tags.nodejs", "tags.#", "4"),
				resource.TestCheckResourceAttr("data.apko_tags.nodejs", "tags.0", "24"),
				resource.TestCheckResourceAttr("data.apko_tags.nodejs", "tags.1", "24.18"),
				resource.TestCheckResourceAttr("data.apko_tags.nodejs", "tags.2", "24.18.0"),
				resource.TestCheckResourceAttr("data.apko_tags.nodejs", "tags.3", "24.18.0-r1"),
				resource.TestCheckResourceAttr("data.apko_tags.nodejs", "id", "24,24.18,24.18.0,24.18.0-r1"),

				resource.TestCheckResourceAttr("data.apko_tags.nodejs-24", "tags.#", "4"),
				resource.TestCheckResourceAttr("data.apko_tags.nodejs-24", "tags.0", "24"),
				resource.TestCheckResourceAttr("data.apko_tags.nodejs-24", "tags.1", "24.18"),
				resource.TestCheckResourceAttr("data.apko_tags.nodejs-24", "tags.2", "24.18.0"),
				resource.TestCheckResourceAttr("data.apko_tags.nodejs-24", "tags.3", "24.18.0-r1"),
				resource.TestCheckResourceAttr("data.apko_tags.nodejs-24", "id", "24,24.18,24.18.0,24.18.0-r1"),

				//24.18.0-r1.apk
			),
		}},
	})
}

func TestAccDataSourceTags_Provides(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories: []string{"https://packages.wolfi.dev/os"},
				keyring:      []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:        []string{"x86_64", "aarch64"},
			}),
		},
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
contents:
  packages:
  # Satisfied by apache-pulsar-4.2-compat, which provides apache-pulsar-compat.
  # The name is not a prefix match, so the tags have to come from 'provides'.
  - apache-pulsar-compat=4.2.2-r0
EOF
}

data "apko_tags" "provided" {
  config         = data.apko_config.this.config
  target_package = "apache-pulsar-compat"
}

data "apko_tags" "provider" {
  config         = data.apko_config.this.config
  target_package = "apache-pulsar-4.2-compat"
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.#", "1"),
				resource.TestCheckResourceAttr("data.apko_config.this", "config.contents.packages.0", "apache-pulsar-4.2-compat=4.2.2-r0"),

				resource.TestCheckResourceAttr("data.apko_tags.provided", "tags.#", "4"),
				resource.TestCheckResourceAttr("data.apko_tags.provided", "tags.0", "4"),
				resource.TestCheckResourceAttr("data.apko_tags.provided", "tags.1", "4.2"),
				resource.TestCheckResourceAttr("data.apko_tags.provided", "tags.2", "4.2.2"),
				resource.TestCheckResourceAttr("data.apko_tags.provided", "tags.3", "4.2.2-r0"),
				resource.TestCheckResourceAttr("data.apko_tags.provided", "id", "4,4.2,4.2.2,4.2.2-r0"),

				resource.TestCheckResourceAttr("data.apko_tags.provider", "id", "4,4.2,4.2.2,4.2.2-r0"),
			),
		}, {
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
contents:
  packages:
  - apache-pulsar-compat=4.2.2-r0
EOF
}

data "apko_tags" "missing" {
  config         = data.apko_config.this.config
  target_package = "pulsar"
}
`,
			ExpectError: regexp.MustCompile("Unable to find package: pulsar"),
		}},
	})
}

func TestProviderOf(t *testing.T) {
	amd64 := apkotypes.ParseArchitecture("x86_64")
	arm64 := apkotypes.ParseArchitecture("aarch64")

	rpkg := func(name, version string, provides ...string) *apk.RepositoryPackage {
		return apk.NewRepositoryPackage(&apk.Package{
			Name:     name,
			Version:  version,
			Provides: provides,
		}, nil)
	}

	tests := []struct {
		name     string
		target   string
		pkgs     map[string]string
		resolved map[apkotypes.Architecture][]*apk.RepositoryPackage
		want     string
		wantErr  bool
	}{{
		name:   "provided with version",
		target: "kubectl",
		pkgs:   map[string]string{"kubernetes-1.28": "1.28.1-r0", "glibc": "2.38-r8"},
		resolved: map[apkotypes.Architecture][]*apk.RepositoryPackage{
			amd64: {
				rpkg("glibc", "2.38-r8", "so:libc.so.6=6"),
				rpkg("kubernetes-1.28", "1.28.1-r0", "cmd:kubectl=1.28.1-r0", "kubectl=1.28.1-r0"),
			},
		},
		want: "kubernetes-1.28",
	}, {
		name:   "provided without version",
		target: "foo-bar",
		pkgs:   map[string]string{"foo-fips-bar": "1.2.3-r0"},
		resolved: map[apkotypes.Architecture][]*apk.RepositoryPackage{
			amd64: {rpkg("foo-fips-bar", "1.2.3-r0", "foo-bar")},
		},
		want: "foo-fips-bar",
	}, {
		name:   "only matches whole names",
		target: "kube",
		pkgs:   map[string]string{"kubernetes-1.28": "1.28.1-r0"},
		resolved: map[apkotypes.Architecture][]*apk.RepositoryPackage{
			amd64: {rpkg("kubernetes-1.28", "1.28.1-r0", "kubectl=1.28.1-r0", "cmd:kube=1.28.1-r0")},
		},
		want: "",
	}, {
		name:   "ignores packages that are not locked in the config",
		target: "kubectl",
		pkgs:   map[string]string{"glibc": "2.38-r8"},
		resolved: map[apkotypes.Architecture][]*apk.RepositoryPackage{
			arm64: {rpkg("kubernetes-1.28", "1.28.1-r0", "kubectl=1.28.1-r0")},
		},
		want: "",
	}, {
		name:   "ignores packages locked to a different version",
		target: "kubectl",
		pkgs:   map[string]string{"kubernetes-1.28": "1.28.2-r0"},
		resolved: map[apkotypes.Architecture][]*apk.RepositoryPackage{
			arm64: {rpkg("kubernetes-1.28", "1.28.1-r0", "kubectl=1.28.1-r0")},
		},
		want: "",
	}, {
		name:   "multiple providers with the same version",
		target: "foo",
		pkgs:   map[string]string{"bar": "1.0-r0", "baz": "1.0-r0"},
		resolved: map[apkotypes.Architecture][]*apk.RepositoryPackage{
			amd64: {rpkg("bar", "1.0-r0", "foo=1.0-r0")},
			arm64: {rpkg("baz", "1.0-r0", "foo=1.0-r0")},
		},
		want: "baz",
	}, {
		name:   "multiple providers with different versions",
		target: "foo",
		pkgs:   map[string]string{"bar": "1.0-r0", "baz": "2.0-r0"},
		resolved: map[apkotypes.Architecture][]*apk.RepositoryPackage{
			amd64: {rpkg("bar", "1.0-r0", "foo=1.0-r0"), rpkg("baz", "2.0-r0", "foo=2.0-r0")},
		},
		wantErr: true,
	}, {
		name:     "nothing resolved",
		target:   "foo",
		pkgs:     map[string]string{"bar": "1.0-r0"},
		resolved: nil,
		want:     "",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := providerOf(tt.target, tt.pkgs, tt.resolved)
			if (err != nil) != tt.wantErr {
				t.Fatalf("providerOf() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("providerOf() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAccDataSourceTags_Disabled(t *testing.T) {
	t.Setenv("TF_APKO_DISABLE_VERSION_TAGS", "anything")

	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories: []string{"https://packages.wolfi.dev/os"},
				keyring:      []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:        []string{"x86_64", "aarch64"},
				packages:     []string{"wolfi-baselayout=20230201-r24"},
			}),
		},
		Steps: []resource.TestStep{{
			Config: `
data "apko_config" "this" {
  config_contents = <<EOF
contents:
  packages:
  - ca-certificates-bundle=20250911-r0
  - glibc-locale-posix=2.42-r2
  - ko=0.19.1-r1
  - nodejs=24.18.0-r1
EOF
  extra_packages = ["tzdata=2025b-r2"]
}

data "apko_tags" "ca-certs" {
  config         = data.apko_config.this.config
  target_package = "ca-certificates-bundle"
}

data "apko_tags" "glibc" {
  config         = data.apko_config.this.config
  target_package = "glibc-locale-posix"
}

data "apko_tags" "wolfi-baselayout" {
  config         = data.apko_config.this.config
  target_package = "wolfi-baselayout"
}

data "apko_tags" "tzdata" {
  config         = data.apko_config.this.config
  target_package = "tzdata"
}

data "apko_tags" "ko" {
  config         = data.apko_config.this.config
  target_package = "ko"
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.apko_tags.glibc", "tags.#", "0"),
				resource.TestCheckResourceAttr("data.apko_tags.glibc", "id", "disabled"),

				resource.TestCheckResourceAttr("data.apko_tags.ca-certs", "tags.#", "0"),
				resource.TestCheckResourceAttr("data.apko_tags.ca-certs", "id", "disabled"),

				resource.TestCheckResourceAttr("data.apko_tags.wolfi-baselayout", "tags.#", "0"),
				resource.TestCheckResourceAttr("data.apko_tags.wolfi-baselayout", "id", "disabled"),

				resource.TestCheckResourceAttr("data.apko_tags.tzdata", "tags.#", "0"),
				resource.TestCheckResourceAttr("data.apko_tags.tzdata", "id", "disabled"),

				resource.TestCheckResourceAttr("data.apko_tags.ko", "tags.#", "0"),
				resource.TestCheckResourceAttr("data.apko_tags.ko", "id", "disabled"),
			),
		}},
	})
}
