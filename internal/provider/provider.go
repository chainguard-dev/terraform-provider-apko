package provider

import (
	"context"
	"maps"
	"runtime/debug"
	"time"

	"chainguard.dev/apko/pkg/apk/apk"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var _ provider.Provider = &Provider{}

type Provider struct {
	version string

	repositories, buildRespositories, packages, keyring, archs []string
	anns                                                       map[string]string
	layering                                                   *LayeringConfig
}

type LayeringConfig struct {
	Strategy string `tfsdk:"strategy"`
	Budget   int    `tfsdk:"budget"`
}

type ProviderModel struct {
	ExtraRepositories  []string          `tfsdk:"extra_repositories"`
	BuildRepositories  []string          `tfsdk:"build_repositories"`
	ExtraPackages      []string          `tfsdk:"extra_packages"`
	ExtraKeyring       []string          `tfsdk:"extra_keyring"`
	DefaultAnnotations map[string]string `tfsdk:"default_annotations"`
	DefaultArchs       []string          `tfsdk:"default_archs"`
	DefaultLayering    *LayeringConfig   `tfsdk:"default_layering"`
	PlanOffline        *bool             `tfsdk:"plan_offline"`
}

type ProviderOpts struct {
	repositories, buildRespositories, packages, keyring, archs []string
	anns                                                       map[string]string
	layering                                                   *LayeringConfig
	cache                                                      *apk.Cache
	ropts                                                      []remote.Option
	planOffline                                                bool
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
			"default_layering": schema.SingleNestedAttribute{
				Description: "Default image layering configuration when not specified in the config",
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"strategy": schema.StringAttribute{
						Description: "Layering strategy, currently only 'origin' is supported",
						Required:    true,
						Validators: []validator.String{
							stringvalidator.OneOf("origin"),
						},
					},
					"budget": schema.Int64Attribute{
						Description: "Budget for the maximum number of layers that can be generated",
						Required:    true,
						Validators: []validator.Int64{
							int64validator.AtLeast(1),
						},
					},
				},
			},
			"plan_offline": schema.BoolAttribute{
				Description: "Whether to plan offline",
				Optional:    true,
			},
		},
	}
}

// combineMaps combines two maps, with the first map (left) taking precedence.
func combineMaps(left, right map[string]string) map[string]string {
	out := map[string]string{}
	maps.Copy(out, right)
	maps.Copy(out, left)
	return out
}

func (p *Provider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data ProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	kc := authn.NewMultiKeychain(google.Keychain, authn.RefreshingKeychain(authn.DefaultKeychain, 30*time.Minute))
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

	// Use the provider's layering configuration if provided through test, otherwise use the config
	var layering *LayeringConfig
	if p.layering != nil {
		layering = p.layering
	} else {
		layering = data.DefaultLayering
	}

	opts := &ProviderOpts{
		// This is only for testing, so we can inject provider config
		repositories:       append(p.repositories, data.ExtraRepositories...),
		buildRespositories: append(p.buildRespositories, data.BuildRepositories...),
		packages:           append(p.packages, data.ExtraPackages...),
		keyring:            append(p.keyring, data.ExtraKeyring...),
		archs:              append(p.archs, data.DefaultArchs...),
		anns:               combineMaps(p.anns, data.DefaultAnnotations),
		layering:           layering,
		cache:              apk.NewCache(true),
		planOffline:        data.PlanOffline != nil && *data.PlanOffline,
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

func (p *Provider) Functions(ctx context.Context) []func() function.Function {
	return []func() function.Function{
		func() function.Function {
			return NewVersionFunction(p.version)
		},
	}
}

// getVersion attempts to get the version from build info.
func getVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		// When built with goreleaser, this will be the actual version
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}
	return "dev"
}

func New() func() provider.Provider {
	return func() provider.Provider {
		return &Provider{
			version: getVersion(),
		}
	}
}
