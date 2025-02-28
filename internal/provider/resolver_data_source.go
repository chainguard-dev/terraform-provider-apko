package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	apko_types "chainguard.dev/apko/pkg/build/types"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"gopkg.in/yaml.v2"
)

var _ datasource.DataSource = &ResolverDataSource{}

func NewResolverDataSource() datasource.DataSource {
	return &ResolverDataSource{}
}

type ResolverDataSource struct {
	popts ProviderOpts
}

type ResolverDataSourceModel struct {
	Source types.String `tfsdk:"source"`
	Out    types.String `tfsdk:"out"`

	Configs  map[string]ResolverDataSourceConfigsModel `tfsdk:"configs"`
	Versions *ResolverDataSourceVersionsModel          `tfsdk:"versions"`

	Resolved map[string]ResolverDataSourceResolvedModel `tfsdk:"resolved"`
}

type ResolverDataSourceResolvedModel struct {
	Resolved          string       `tfsdk:"resolved"`
	Locked            types.Object `tfsdk:"locked"`
	VersionStreamName string       `tfsdk:"version_stream_name"`
	Component         string       `tfsdk:"component"`

	Tags []string `tfsdk:"tags"`

	// Identical to ResolverdataSourceVersionsversionsModel to better support
	// threading output of this data source
	Eol         bool   `tfsdk:"eol"`
	EolDate     string `tfsdk:"eol_date"`
	Exists      bool   `tfsdk:"exists"`
	Fips        bool   `tfsdk:"fips"`
	IsLatest    bool   `tfsdk:"is_latest"`
	Lts         string `tfsdk:"lts"`
	Main        string `tfsdk:"main"`
	ReleaseDate string `tfsdk:"release_date"`
	Version     string `tfsdk:"version"`
}

type ResolverDataSourceConfigsModel struct {
	Config    string       `tfsdk:"config"`
	Locked    types.Object `tfsdk:"locked"`
	Component string       `tfsdk:"component"`
	Main      string       `tfsdk:"main"`
}

type ResolverDataSourceVersionsModel struct {
	OrderedKeys []string                                           `tfsdk:"ordered_keys"`
	Versions    map[string]ResolverDataSourceVersionsVersionsModel `tfsdk:"versions"`
}

type ResolverDataSourceVersionsVersionsModel struct {
	Eol         bool   `tfsdk:"eol"`
	EolDate     string `tfsdk:"eol_date"`
	Exists      bool   `tfsdk:"exists"`
	Fips        bool   `tfsdk:"fips"`
	IsLatest    bool   `tfsdk:"is_latest"`
	Lts         string `tfsdk:"lts"`
	Main        string `tfsdk:"main"`
	ReleaseDate string `tfsdk:"release_date"`
	Version     string `tfsdk:"version"`
}

// Metadata implements datasource.DataSource.
func (r *ResolverDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resolver"
}

// Schema implements datasource.DataSource.
func (r *ResolverDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "",
		Attributes: map[string]schema.Attribute{
			"source": schema.StringAttribute{
				MarkdownDescription: "Optionally source apko's from a json file.",
				Optional:            true,
			},
			"out": schema.StringAttribute{
				MarkdownDescription: "",
				Optional:            true,
			},
			"configs": schema.MapNestedAttribute{
				MarkdownDescription: "",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"config": schema.StringAttribute{
							MarkdownDescription: "",
							Optional:            true,
						},
						"locked": schema.ObjectAttribute{
							MarkdownDescription: "The parsed structure of the apko configuration.",
							Optional:            true,
							AttributeTypes:      imageConfigurationSchema.AttrTypes,
						},
						"component": schema.StringAttribute{
							MarkdownDescription: "",
							Optional:            true,
						},
						"main": schema.StringAttribute{
							MarkdownDescription: "",
							Optional:            true,
						},
					},
				},
			},
			"versions": schema.SingleNestedAttribute{
				MarkdownDescription: "",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"ordered_keys": schema.ListAttribute{
						MarkdownDescription: "",
						Optional:            true,
						ElementType:         types.StringType,
					},
					"versions": schema.MapNestedAttribute{
						Description: "",
						Optional:    true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"eol": schema.BoolAttribute{
									Description: "Whether the version is eol.",
									Optional:    true,
								},
								"eol_date": schema.StringAttribute{
									Description: "The eol date.",
									Optional:    true,
								},
								"exists": schema.BoolAttribute{
									Description: "Whether the version exists.",
									Optional:    true,
								},
								"fips": schema.BoolAttribute{
									Description: "Whether the version is fips.",
									Optional:    true,
								},
								"is_latest": schema.BoolAttribute{
									Description: "Whether the version is the latest.",
									Optional:    true,
								},
								"lts": schema.StringAttribute{
									Description: "The lts version.",
									Optional:    true,
								},
								"main": schema.StringAttribute{
									Description: "The main version.",
									Optional:    true,
								},
								"release_date": schema.StringAttribute{
									Description: "The release date.",
									Optional:    true,
								},
								"version": schema.StringAttribute{
									Description: "The version.",
									Optional:    true,
								},
							},
						},
					},
				},
			},
			"resolved": schema.MapNestedAttribute{
				Description: "",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"resolved": schema.StringAttribute{
							Description: "The resolved config.",
							Computed:    true,
						},
						"locked": schema.ObjectAttribute{
							MarkdownDescription: "The parsed structure of the apko configuration.",
							Computed:            true,
							Optional:            true,
							AttributeTypes:      imageConfigurationSchema.AttrTypes,
						},
						"version_stream_name": schema.StringAttribute{
							Description: "The version stream name.",
							Computed:    true,
							Optional:    true,
						},
						"component": schema.StringAttribute{
							Description: "The component name.",
							Computed:    true,
							Optional:    true,
						},
						"tags": schema.ListAttribute{
							Description: "The tags for the target package.",
							Computed:    true,
							Optional:    true,
							ElementType: basetypes.StringType{},
						},
						"eol": schema.BoolAttribute{
							Description: "Whether the version is eol.",
							Computed:    true,
							Optional:    true,
						},
						"eol_date": schema.StringAttribute{
							Description: "The eol date.",
							Computed:    true,
							Optional:    true,
						},
						"exists": schema.BoolAttribute{
							Description: "Whether the version exists.",
							Computed:    true,
							Optional:    true,
						},
						"fips": schema.BoolAttribute{
							Description: "Whether the version is fips.",
							Computed:    true,
							Optional:    true,
						},
						"is_latest": schema.BoolAttribute{
							Description: "Whether the version is the latest.",
							Computed:    true,
							Optional:    true,
						},
						"lts": schema.StringAttribute{
							Description: "The lts version.",
							Computed:    true,
							Optional:    true,
						},
						"main": schema.StringAttribute{
							Description: "The main version.",
							Computed:    true,
							Optional:    true,
						},
						"release_date": schema.StringAttribute{
							Description: "The release date.",
							Computed:    true,
							Optional:    true,
						},
						"version": schema.StringAttribute{
							Description: "The version.",
							Computed:    true,
							Optional:    true,
						},
					},
				},
			},
		},
	}
}

// Read implements datasource.DataSource.
func (r *ResolverDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ResolverDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.Out.ValueString() != "" {
		if err := os.MkdirAll(data.Out.ValueString(), 0755); err != nil {
			resp.Diagnostics.AddError("failed to create output directory", fmt.Sprintf("failed to create output directory: %s", err))
			return
		}
	}

	var (
		resolved map[string]ResolverDataSourceResolvedModel
		err      error
	)

	if data.Source.ValueString() != "" {
		resolved, err = r.doSource(ctx, data)
	} else {
		resolved, err = r.do(ctx, data)
	}

	if err != nil {
		resp.Diagnostics.AddError("failed to resolve", fmt.Sprintf("failed to resolve: %s", err))
		return
	}

	data.Resolved = resolved

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ResolverDataSource) do(ctx context.Context, data ResolverDataSourceModel) (map[string]ResolverDataSourceResolvedModel, error) {
	resolved := map[string]ResolverDataSourceResolvedModel{}

	for cn, c := range data.Configs {
		for vsn, vs := range data.Versions.Versions {
			// TODO: This is a pretty bad way to map components to their version
			// stream name, but I think it always works.
			if !strings.Contains(cn, vs.Version) {
				continue
			}

			main := vs.Main
			if c.Main != "" {
				main = c.Main
			}

			var ic apko_types.ImageConfiguration
			if err := json.Unmarshal([]byte(c.Config), &ic); err != nil {
				return nil, fmt.Errorf("failed to unmarshal config: %w", err)
			}

			raw, err := json.Marshal(ic)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal config: %w", err)
			}

			var tags []string

			if data.Out.ValueString() != "" {
				if err := os.MkdirAll(data.Out.ValueString(), 0755); err != nil {
					return nil, fmt.Errorf("failed to create output directory: %w", err)
				}

				// Write yaml instead of json since we expect these to be human readable
				raw, err := yaml.Marshal(ic)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal config: %w", err)
				}

				p := filepath.Join(data.Out.ValueString(), fmt.Sprintf("%s.%s.resolved.yaml", vsn, cn))
				if err := os.WriteFile(p, raw, 0644); err != nil {
					return nil, fmt.Errorf("failed to write config: %w", err)
				}

				if !c.Locked.IsNull() {
					var lic apko_types.ImageConfiguration
					if diags := assignValue(c.Locked, &lic); diags.HasError() {
						return nil, fmt.Errorf("failed to assign value:")
					}

					lp := filepath.Join(data.Out.ValueString(), fmt.Sprintf("%s.%s.locked.yaml", vsn, cn))
					raw, err := yaml.Marshal(lic)
					if err != nil {
						return nil, fmt.Errorf("failed to marshal config: %w", err)
					}
					if err := os.WriteFile(lp, raw, 0644); err != nil {
						return nil, fmt.Errorf("failed to write config: %w", err)
					}

					// If we have locks, we can get tags
					version, diags := getPkgVers(lic, main)
					if diags.HasError() {
						return nil, fmt.Errorf("failed to get package version: %v", diags.Errors())
					}

					tags = getStemmedVersionTags(version)
					sort.Strings(tags)

					if vs.IsLatest {
						tags = append(tags, "latest")
					}
				}
			}

			resolved[cn] = ResolverDataSourceResolvedModel{
				Resolved:          string(raw),
				Locked:            c.Locked,
				VersionStreamName: vsn,
				Component:         c.Component,
				Tags:              tags,

				// Repeated fields from versions input
				Main:        main,
				Eol:         vs.Eol,
				EolDate:     vs.EolDate,
				Exists:      vs.Exists,
				Fips:        vs.Fips,
				IsLatest:    vs.IsLatest,
				Lts:         vs.Lts,
				ReleaseDate: vs.ReleaseDate,
				Version:     vs.Version,
			}
		}
	}

	return resolved, nil
}

func (r *ResolverDataSource) doSource(ctx context.Context, data ResolverDataSourceModel) (map[string]ResolverDataSourceResolvedModel, error) {
	resolved := map[string]ResolverDataSourceResolvedModel{}

	// Short circuit and get things from a file
	f, err := os.Open(data.Source.ValueString())
	if err != nil {
		return nil, fmt.Errorf("failed to open source: %w", err)
	}
	defer f.Close()

	var source ResolverSource
	if err := json.NewDecoder(f).Decode(&source); err != nil {
		return nil, fmt.Errorf("failed to decode source: %w", err)
	}

	for cn, c := range source.ImageLocks {
		var dammitAlex struct {
			Config apko_types.ImageConfiguration
		}
		if err := json.Unmarshal([]byte(c.Configs["index"]), &dammitAlex); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}

		v, ok := data.Versions.Versions[cn]
		if !ok {
			return nil, fmt.Errorf("no matching versions tream for resolved config component: %w", err)
		}

		raw, err := json.Marshal(dammitAlex.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}

		resolved[cn] = ResolverDataSourceResolvedModel{
			Resolved: string(raw),
			Locked:   types.ObjectNull(imageConfigurationSchema.AttrTypes),

			Main:        c.Main,
			Eol:         v.Eol,
			EolDate:     v.EolDate,
			Exists:      v.Exists,
			Fips:        v.Fips,
			IsLatest:    v.IsLatest,
			Lts:         v.Lts,
			ReleaseDate: v.ReleaseDate,
			Version:     v.Version,
		}
	}

	return resolved, nil
}

type ResolverSource struct {
	ImageLocks map[string]ResolverSourceConfigs `json:"imageLocks"`
}

type ResolverSourceConfigs struct {
	Configs map[string]string `json:"configs"`
	Eol     bool              `json:"eol"`
	Main    string            `json:"main"`
	Tags    []string          `json:"tags"`
	Latest  bool              `json:"latest"`
}
