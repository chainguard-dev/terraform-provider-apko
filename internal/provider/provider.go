package provider

import (
	"context"

	"chainguard.dev/apko/pkg/apk/apk"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var _ provider.Provider = &Provider{}

type Provider struct {
	version string

	repositories, buildRespositories, packages, keyring, archs []string
	anns                                                       map[string]string
}

type ProviderModel struct {
	ExtraRepositories  []string          `tfsdk:"extra_repositories"`
	BuildRepositories  []string          `tfsdk:"build_repositories"`
	ExtraPackages      []string          `tfsdk:"extra_packages"`
	ExtraKeyring       []string          `tfsdk:"extra_keyring"`
	DefaultAnnotations map[string]string `tfsdk:"default_annotations"`
	DefaultArchs       []string          `tfsdk:"default_archs"`
}

type ProviderOpts struct {
	repositories, buildRespositories, packages, keyring, archs []string
	anns                                                       map[string]string
	cache                                                      *apk.Cache
	ropts                                                      []remote.Option
}

func (p *Provider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "apko"
	resp.Version = p.version
}

func (p *Provider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"extra_repositories": schema.ListAttribute{
				Description: "Additional repositories to search for packages",
				Optional:    true,
				ElementType: basetypes.StringType{},
			},
			"build_repositories": schema.ListAttribute{
				Description: "Additional repositories to search for packages, only during apko build",
				Optional:    true,
				ElementType: basetypes.StringType{},
			},
			"extra_packages": schema.ListAttribute{
				Description: "Additional packages to install",
				Optional:    true,
				ElementType: basetypes.StringType{},
			},
			"extra_keyring": schema.ListAttribute{
				Description: "Additional keys to use for package verification",
				Optional:    true,
				ElementType: basetypes.StringType{},
			},
			"default_annotations": schema.MapAttribute{
				Description: "Default annotations to add",
				Optional:    true,
				ElementType: basetypes.StringType{},
			},
			"default_archs": schema.ListAttribute{
				Description: "Default architectures to build for",
				Optional:    true,
				ElementType: basetypes.StringType{},
			},
		},
	}
}

// combineMaps combines two maps, with the first map (left) taking precedence.
func combineMaps(left, right map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range right {
		out[k] = v
	}
	for k, v := range left {
		out[k] = v
	}
	return out
}

func (p *Provider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data ProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	kc := authn.NewMultiKeychain(google.Keychain, authn.DefaultKeychain)
	ropts := []remote.Option{
		remote.WithAuthFromKeychain(kc),
		remote.WithUserAgent("terraform-provider-apko/" + p.version),
	}

	puller, err := remote.NewPuller(ropts...)
	if err != nil {
		resp.Diagnostics.AddError("Configure []remote.Option", err.Error())
		return
	}
	pusher, err := remote.NewPusher(ropts...)
	if err != nil {
		resp.Diagnostics.AddError("Configure []remote.Option", err.Error())
		return
	}
	ropts = append(ropts, remote.Reuse(puller), remote.Reuse(pusher))

	opts := &ProviderOpts{
		// This is only for testing, so we can inject provider config
		repositories:       append(p.repositories, data.ExtraRepositories...),
		buildRespositories: append(p.buildRespositories, data.BuildRepositories...),
		packages:           append(p.packages, data.ExtraPackages...),
		keyring:            append(p.keyring, data.ExtraKeyring...),
		archs:              append(p.archs, data.DefaultArchs...),
		anns:               combineMaps(p.anns, data.DefaultAnnotations),
		cache:              apk.NewCache(true),
		ropts:              ropts,
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
		NewTagsDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &Provider{
			version: version,
		}
	}
}
