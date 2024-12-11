package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"chainguard.dev/apko/pkg/build"
	apkotypes "chainguard.dev/apko/pkg/build/types"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &ConfigDataSource{}

func NewConfigDataSource() datasource.DataSource {
	return &ConfigDataSource{}
}

// ConfigDataSource defines the data source implementation.
type ConfigDataSource struct {
	popts ProviderOpts
}

// ConfigDataSourceModel describes the data source data model.
type ConfigDataSourceModel struct {
	Id                 types.String      `tfsdk:"id"`
	ConfigContents     types.String      `tfsdk:"config_contents"`
	Config             types.Object      `tfsdk:"config"`
	Configs            types.Map         `tfsdk:"configs"`
	ExtraPackages      []string          `tfsdk:"extra_packages"`
	DefaultAnnotations map[string]string `tfsdk:"default_annotations"`
}

var imageConfigurationSchema basetypes.ObjectType
var imageConfigurationsSchema basetypes.ObjectType

func init() {
	sch, err := generateType(apkotypes.ImageConfiguration{})
	if err != nil {
		panic(err)
	}

	var ok bool
	imageConfigurationSchema, ok = sch.(basetypes.ObjectType)
	if !ok {
		panic("expected object type")
	}

	imageConfigurationsSchema = basetypes.ObjectType{
		AttrTypes: map[string]attr.Type{
			"config": imageConfigurationSchema,
		},
	}
}

func (d *ConfigDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_config"
}

func (d *ConfigDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This reads an apko configuration file into a structured form.",
		Attributes: map[string]schema.Attribute{
			"config_contents": schema.StringAttribute{
				MarkdownDescription: "The raw contents of the apko configuration.",
				Optional:            true,
			},
			"config": schema.ObjectAttribute{
				MarkdownDescription: "The parsed structure of the apko configuration.",
				Computed:            true,
				AttributeTypes:      imageConfigurationSchema.AttrTypes,
			},
			"configs": schema.MapNestedAttribute{
				MarkdownDescription: "A map from the APK architecture to the config for that architecture.",
				Computed:            true,
				Optional:            true,
				Required:            false,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"config": schema.ObjectAttribute{
							MarkdownDescription: "The parsed structure of the apko configuration.",
							Computed:            true,
							AttributeTypes:      imageConfigurationSchema.AttrTypes,
						},
					},
				},
			},
			"extra_packages": schema.ListAttribute{
				MarkdownDescription: "A list of extra packages to install.",
				Optional:            true,
				ElementType:         basetypes.StringType{},
			},
			"default_annotations": schema.MapAttribute{
				MarkdownDescription: "Default annotations to add.",
				Optional:            true,
				ElementType:         basetypes.StringType{},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for this apko config.",
				Computed:            true,
			},
		},
	}
}

func (d *ConfigDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	popts, ok := req.ProviderData.(*ProviderOpts)
	if !ok || popts == nil {
		resp.Diagnostics.AddError("Client Error", "invalid provider data")
		return
	}
	d.popts = *popts
}

func (d *ConfigDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ConfigDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var ic apkotypes.ImageConfiguration
	if err := yaml.UnmarshalStrict([]byte(data.ConfigContents.ValueString()), &ic); err != nil {
		resp.Diagnostics.AddError("Unable to parse apko configuration", err.Error())
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("got repos: %v", d.popts.repositories))
	tflog.Trace(ctx, fmt.Sprintf("got build repos: %v", d.popts.buildRespositories))
	tflog.Trace(ctx, fmt.Sprintf("got keyring: %v", d.popts.keyring))

	// Append any provider-specified repositories, packages, and keys, if specified.
	ic.Contents.RuntimeRepositories = sets.List(sets.New(ic.Contents.RuntimeRepositories...).Insert(d.popts.repositories...))
	ic.Contents.BuildRepositories = sets.List(sets.New(ic.Contents.BuildRepositories...).Insert(d.popts.buildRespositories...))
	ic.Contents.Packages = sets.List(sets.New(ic.Contents.Packages...).Insert(d.popts.packages...))
	ic.Contents.Keyring = sets.List(sets.New(ic.Contents.Keyring...).Insert(d.popts.keyring...))

	// Append any extra packages specified in the data source configuration.
	ic.Contents.Packages = sets.List(sets.New(ic.Contents.Packages...).Insert(data.ExtraPackages...))

	// Append any extra annotations specified in the data source or provider configuration.
	// The YAML config takes precedence, then the data source config, then the provider config.
	ic.Annotations = combineMaps(ic.Annotations, combineMaps(data.DefaultAnnotations, d.popts.anns))

	// Default to the provider architectures when the image configuration
	// doesn't specify any.
	if len(ic.Archs) == 0 {
		if len(d.popts.archs) != 0 {
			ic.Archs = apkotypes.ParseArchitectures(d.popts.archs)
		} else {
			// Default to all archs when provider and config data source don't specify any.
			ic.Archs = apkotypes.AllArchs
		}
	}

	// Normalize the architectures we surface
	for i, a := range ic.Archs {
		ic.Archs[i] = apkotypes.ParseArchitecture(a.ToAPK())
	}

	input, err := yaml.Marshal(ic)
	if err != nil {
		resp.Diagnostics.AddError("Unable to marshal apko configuration", err.Error())
		return
	}

	h := sha256.Sum256(input)
	hash := hex.EncodeToString(h[:])

	if out := os.Getenv("TF_APKO_OUT_DIR"); out != "" {
		if err := writeFile(out, hash, "pre", ic); err != nil {
			resp.Diagnostics.AddError("Unable to write apko configuration", err.Error())
			return
		}
	}

	// Resolve the package list to specific versions (as much as we can with
	// multi-arch), and overwrite the package list in the ImageConfiguration.
	pls, diags := d.resolvePackageList(ctx, ic)
	resp.Diagnostics = append(resp.Diagnostics, diags...)
	if diags.HasError() {
		return
	}

	cfgMap := make(map[string]attr.Value)

	for arch, ic := range pls {
		ov, diags := generateValue(*ic)
		resp.Diagnostics = append(resp.Diagnostics, diags...)
		if diags.HasError() {
			return
		}

		cfg, ok := ov.(basetypes.ObjectValue)
		if !ok {
			resp.Diagnostics.AddError("Unable to write apko configuration", "unexpected object type or malformed object type")
			return
		}

		// Keep original behavior for "apko_config.config" that only uses only the merged "index" arch.
		if arch == "index" {
			if out := os.Getenv("TF_APKO_OUT_DIR"); out != "" {
				if err := writeFile(out, hash, "post", *ic); err != nil {
					resp.Diagnostics.AddError("Unable to write apko configuration", err.Error())
					return
				}
			}

			data.Config = cfg
		}

		val, diags := types.ObjectValue(imageConfigurationsSchema.AttrTypes, map[string]attr.Value{
			"config": cfg,
		})
		resp.Diagnostics = append(resp.Diagnostics, diags...)
		if diags.HasError() {
			return
		}

		cfgMap[arch] = val
	}

	cfgMapValue, diags := types.MapValue(imageConfigurationsSchema, cfgMap)
	if diags != nil {
		resp.Diagnostics = append(resp.Diagnostics, diags...)
		return
	}
	data.Configs = cfgMapValue

	data.Id = types.StringValue(hash)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func writeFile(dir, hash, variant string, ic apkotypes.ImageConfiguration) error {
	if err := os.MkdirAll(dir, 0644); err != nil {
		return err
	}
	b, err := json.MarshalIndent(ic, "", "  ")
	if err != nil {
		return err
	}
	fn := fmt.Sprintf("%s.%s.apko.json", hash[0:6], variant)
	return os.WriteFile(filepath.Join(dir, fn), b, 0644)
}

func (d *ConfigDataSource) resolvePackageList(ctx context.Context, ic apkotypes.ImageConfiguration) (map[string]*apkotypes.ImageConfiguration, diag.Diagnostics) {
	_, ic2, err := fromImageData(ctx, ic, d.popts)
	if err != nil {
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("Unable to parse apko config", err.Error())}
	}

	pls, missingByArch, err := build.LockImageConfiguration(ctx, *ic2,
		build.WithCache("", false, d.popts.cache),
		build.WithSBOMFormats([]string{"spdx"}),
		build.WithExtraKeys(d.popts.keyring),
		build.WithExtraBuildRepos(d.popts.buildRespositories),
		build.WithExtraRuntimeRepos(d.popts.repositories))
	if err != nil {
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("computing package locks", err.Error())}
	}

	var diagnostics diag.Diagnostics

	for arch, missing := range missingByArch {
		diagnostics = append(diagnostics, diag.NewWarningDiagnostic(
			fmt.Sprintf("unable to lock certain packages for %s", arch),
			fmt.Sprint(missing),
		))
	}

	return pls, diagnostics
}
