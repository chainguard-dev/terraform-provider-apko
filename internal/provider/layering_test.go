package provider

import (
	"fmt"
	"testing"

	ocitesting "github.com/chainguard-dev/terraform-provider-oci/testing"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestProviderLayeringDefaults tests that provider-level layering settings are applied
// when no layering is specified at the resource level.
func TestProviderLayeringDefaults(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test-provider-layering")
	defer cleanup()

	repoStr := repo.String()

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories:       []string{"https://packages.wolfi.dev/os"},
				buildRespositories: []string{"./packages"},
				keyring:            []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:              []string{"x86_64"},
				packages:           []string{"wolfi-baselayout"},
				layering: &LayeringConfig{
					Strategy: "origin",
					Budget:   5, // Provider sets a budget of 5
				},
			}),
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "apko_config" "provider_layering" {
  config_contents = <<EOF
contents:
  packages:
    - ca-certificates-bundle=20250911-r0
    - glibc-locale-posix=2.42-r2
    - tzdata=2025b-r2
# No layering configuration specified - should use provider defaults
EOF
}

output "layering_strategy" {
  value = data.apko_config.provider_layering.config.layering.strategy
}

output "layering_budget" {
  value = data.apko_config.provider_layering.config.layering.budget
}

resource "apko_build" "provider_layering" {
  repo   = %q
  config = data.apko_config.provider_layering.config
}
`, repoStr),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckOutput("layering_strategy", "origin"),
					resource.TestCheckOutput("layering_budget", "5"),
					resource.TestCheckFunc(func(s *terraform.State) error {
						// Get the image reference from terraform state
						rs, ok := s.RootModule().Resources["apko_build.provider_layering"]
						if !ok {
							return fmt.Errorf("resource not found: apko_build.provider_layering")
						}

						imageRef := rs.Primary.Attributes["image_ref"]
						if imageRef == "" {
							return fmt.Errorf("no image_ref in resource state")
						}

						// Pull the image and check its layers
						img, err := crane.Pull(imageRef)
						if err != nil {
							return fmt.Errorf("failed to pull image: %v", err)
						}

						manifest, err := img.Manifest()
						if err != nil {
							return fmt.Errorf("failed to get manifest: %v", err)
						}

						// We expect multiple layers because provider layering is set
						// With 4 packages (3 from config + wolfi-baselayout) and a budget of 5,
						// we should expect each package to get its own layer, plus the metadata layer
						expectedLayerCount := 5 // 4 packages + 1 metadata layer

						actualLayerCount := len(manifest.Layers)
						if actualLayerCount != expectedLayerCount {
							return fmt.Errorf("expected %d layers from provider-level layering with budget 5, got %d layers",
								expectedLayerCount, actualLayerCount)
						}

						return nil
					}),
				),
			},
		},
	})
}

// TestResourceLayeringOverrides tests that resource-level layering settings override
// the provider-level defaults.
func TestResourceLayeringOverrides(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test-resource-layering")
	defer cleanup()

	repoStr := repo.String()

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories:       []string{"https://packages.wolfi.dev/os"},
				buildRespositories: []string{"./packages"},
				keyring:            []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:              []string{"x86_64"},
				packages:           []string{"wolfi-baselayout"},
				layering: &LayeringConfig{
					Strategy: "origin",
					Budget:   1, // Provider sets a very low budget
				},
			}),
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "apko_config" "resource_layering" {
  config_contents = <<EOF
contents:
  packages:
    - ca-certificates-bundle=20250911-r0
    - glibc-locale-posix=2.42-r2
    - tzdata=2025b-r2
layering:
  strategy: origin
  budget: 10  # Resource explicitly sets a higher budget, should override provider
EOF
}

output "resource_layering_strategy" {
  value = data.apko_config.resource_layering.config.layering.strategy
}

output "resource_layering_budget" {
  value = data.apko_config.resource_layering.config.layering.budget
}

resource "apko_build" "resource_layering" {
  repo    = %q
  config  = data.apko_config.resource_layering.config
  configs = data.apko_config.resource_layering.configs
}
`, repoStr),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckOutput("resource_layering_strategy", "origin"),
					resource.TestCheckOutput("resource_layering_budget", "10"),
					resource.TestCheckFunc(func(s *terraform.State) error {
						// Get the image reference from terraform state
						rs, ok := s.RootModule().Resources["apko_build.resource_layering"]
						if !ok {
							return fmt.Errorf("resource not found: apko_build.resource_layering")
						}

						imageRef := rs.Primary.Attributes["image_ref"]
						if imageRef == "" {
							return fmt.Errorf("no image_ref in resource state")
						}

						// Pull the image and check its layers
						img, err := crane.Pull(imageRef)
						if err != nil {
							return fmt.Errorf("failed to pull image: %v", err)
						}

						manifest, err := img.Manifest()
						if err != nil {
							return fmt.Errorf("failed to get manifest: %v", err)
						}

						// With resource layering set to budget 10, we should get multiple layers.
						// If the provider setting (budget 1) was being used instead, we'd only get 2 layers.
						// With 4 packages (3 from config + wolfi-baselayout) and a budget of 10,
						// we should expect each package to get its own layer, plus the metadata layer
						expectedLayerCount := 5 // 4 packages + 1 metadata layer

						actualLayerCount := len(manifest.Layers)
						if actualLayerCount != expectedLayerCount {
							return fmt.Errorf("expected %d layers from resource-level layering with budget 10, got %d layers",
								expectedLayerCount, actualLayerCount)
						}

						return nil
					}),
				),
			},
		},
	})
}

// TestEmptyLayeringOverride tests that an empty layering block (layering: {})
// at the resource level overrides the provider-level default_layering,
// resulting in a single-layer image.
func TestEmptyLayeringOverride(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test-empty-layering")
	defer cleanup()

	repoStr := repo.String()

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"apko": providerserver.NewProtocol6WithError(&Provider{
				repositories:       []string{"https://packages.wolfi.dev/os"},
				buildRespositories: []string{"./packages"},
				keyring:            []string{"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"},
				archs:              []string{"x86_64"},
				packages:           []string{"wolfi-baselayout"},
				layering: &LayeringConfig{
					Strategy: "origin",
					Budget:   10, // Provider sets a high budget to ensure multiple layers by default
				},
			}),
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "apko_config" "empty_layering" {
  config_contents = <<EOF
contents:
  packages:
    - ca-certificates-bundle=20250911-r0
    - glibc-locale-posix=2.42-r2
    - tzdata=2025b-r2
layering: {}  # Empty layering block should override provider defaults
EOF
}

output "empty_layering_strategy" {
  value = try(data.apko_config.empty_layering.config.layering.strategy, "")
}

output "empty_layering_budget" {
  value = try(data.apko_config.empty_layering.config.layering.budget, 0)
}

resource "apko_build" "empty_layering" {
  repo   = %q
  config = data.apko_config.empty_layering.config
}
`, repoStr),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckOutput("empty_layering_strategy", ""),
					resource.TestCheckOutput("empty_layering_budget", "0"),
					resource.TestCheckFunc(func(s *terraform.State) error {
						// Get the image reference from terraform state
						rs, ok := s.RootModule().Resources["apko_build.empty_layering"]
						if !ok {
							return fmt.Errorf("resource not found: apko_build.empty_layering")
						}

						imageRef := rs.Primary.Attributes["image_ref"]
						if imageRef == "" {
							return fmt.Errorf("no image_ref in resource state")
						}

						// Pull the image and check its layers
						img, err := crane.Pull(imageRef)
						if err != nil {
							return fmt.Errorf("failed to pull image: %v", err)
						}

						manifest, err := img.Manifest()
						if err != nil {
							return fmt.Errorf("failed to get manifest: %v", err)
						}

						// With an empty layering block, the provider defaults should be ignored
						// and we should get a single layer for the image (no layering)
						expectedLayerCount := 1

						actualLayerCount := len(manifest.Layers)
						if actualLayerCount != expectedLayerCount {
							return fmt.Errorf("expected %d layer from empty layering override, got %d layers",
								expectedLayerCount, actualLayerCount)
						}

						return nil
					}),
				),
			},
		},
	})
}
