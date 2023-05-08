package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var _ provider.Provider = &Provider{}

type Provider struct {
	version string

	repositories, packages, keyring, archs []string
}

type ProviderModel struct {
	Repositories []string `tfsdk:"repositories"`
	Packages     []string `tfsdk:"packages"`
	Keyring      []string `tfsdk:"keyring"`
	Archs        []string `tfsdk:"archs"`
}

type ProviderOpts struct {
	repositories, packages, keyring, archs []string
}

func (p *Provider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "apko"
	resp.Version = p.version
}

func (p *Provider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"repositories": schema.ListAttribute{
				Description: "Additional repositories to search for packages",
				Optional:    true,
				ElementType: basetypes.StringType{},
			},
			"packages": schema.ListAttribute{
				Description: "Additional packages to install",
				Optional:    true,
				ElementType: basetypes.StringType{},
			},
			"keyring": schema.ListAttribute{
				Description: "Additional keys to use for package verification",
				Optional:    true,
				ElementType: basetypes.StringType{},
			},
			"archs": schema.ListAttribute{
				Description: "Default architectures to build for",
				Optional:    true,
				ElementType: basetypes.StringType{},
			},
		},
	}
}

func (p *Provider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data ProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	opts := &ProviderOpts{
		// This is only for testing, so we can inject provider config
		repositories: append(p.repositories, data.Repositories...),
		packages:     append(p.packages, data.Packages...),
		keyring:      append(p.keyring, data.Keyring...),
		archs:        append(p.archs, data.Archs...),
	}

	// Make provider opts available to resources and data sources.
	resp.ResourceData = opts
	resp.DataSourceData = opts
}

func (p *Provider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewBuildResource,
	}
}

func (p *Provider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewConfigDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &Provider{
			version: version,
		}
	}
}
