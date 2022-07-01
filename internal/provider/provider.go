package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func init() {
	// Set descriptions to support markdown syntax, this will be used in document generation
	// and the language server.
	schema.DescriptionKind = schema.StringMarkdown

	// Customize the content of descriptions when output. For example you can add defaults on
	// to the exported descriptions if present.
	// schema.SchemaDescriptionBuilder = func(s *schema.Schema) string {
	// 	desc := s.Description
	// 	if s.Default != nil {
	// 		desc += fmt.Sprintf(" Defaults to `%v`.", s.Default)
	// 	}
	// 	return strings.TrimSpace(desc)
	// }
}

func New(version string) func() *schema.Provider {
	return func() *schema.Provider {
		p := &schema.Provider{
			Schema: map[string]*schema.Schema{
				"docker_repo": {
					Description: "Container repositor to publish images to. Defaults to `KO_DOCKER_REPO` env var",
					Optional:    true,
					Type:        schema.TypeString,
				},
			},
			ResourcesMap: map[string]*schema.Resource{
				"apko_image": resourceApkoImage(),
			},
		}

		p.ConfigureContextFunc = configure(version, p)

		return p
	}
}

func configure(version string, p *schema.Provider) func(context.Context, *schema.ResourceData) (interface{}, diag.Diagnostics) {
	return func(ctx context.Context, s *schema.ResourceData) (interface{}, diag.Diagnostics) {
		repo, ok := s.Get("docker_repo").(string)
		if !ok {
			return nil, diag.Errorf("expected docker_repo to be string")
		}
		if repo == "" {
			return "TODO", nil // TODO: fail here, but set it in tests.
		}
		return repo, nil
	}
}
