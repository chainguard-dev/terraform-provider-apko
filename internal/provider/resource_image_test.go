package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccResourceApkoImage(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: `
				resource "apko_image" "foo" {
				  sample_attribute = "bar"
				}
				`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"apko_image.foo", "sample_attribute", regexp.MustCompile("^ba")),
					resource.TestMatchResourceAttr(
						"apko_image.foo", "image_ref", regexp.MustCompile("^TODO$")),
				),
			},
		},
	})
}
