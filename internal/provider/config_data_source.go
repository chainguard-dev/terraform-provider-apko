package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	apkotypes "chainguard.dev/apko/pkg/build/types"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"gopkg.in/yaml.v2"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &ConfigDataSource{}

func NewConfigDataSource() datasource.DataSource {
	return &ConfigDataSource{}
}

// ConfigDataSource defines the data source implementation.
type ConfigDataSource struct {
}

// ConfigDataSourceModel describes the data source data model.
type ConfigDataSourceModel struct {
	Id             types.String        `tfsdk:"id"`
	ConfigContents types.String        `tfsdk:"config_contents"`
	Config         *ImageConfiguration `tfsdk:"config"`
}

type ImageConfiguration struct {
	// TODO(mattmoor): Add the rest of the fields
	// from types.ImageConfiguration
	Archs []types.String `tfsdk:"archs"`
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
				Required:            true,
			},
			"config": schema.ObjectAttribute{
				MarkdownDescription: "The parsed structure of the apko configuration.",
				Computed:            true,
				AttributeTypes: map[string]attr.Type{
					"archs": basetypes.ListType{
						ElemType: types.StringType,
					},
					// TODO(mattmoor): Add the rest of the fields
					// from types.ImageConfiguration
				},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for this apko config.",
				Computed:            true,
			},
		},
	}
}

func (d *ConfigDataSource) Configure(context.Context, datasource.ConfigureRequest, *datasource.ConfigureResponse) {
}

func (d *ConfigDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ConfigDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var ic apkotypes.ImageConfiguration
	if err := yaml.Unmarshal([]byte(data.ConfigContents.ValueString()), &ic); err != nil {
		resp.Diagnostics.AddError("Unable to parse apko configuration", err.Error())
		return
	}

	data.Config = &ImageConfiguration{
		Archs: make([]basetypes.StringValue, 0, len(ic.Archs)),
	}

	for _, arch := range ic.Archs {
		data.Config.Archs = append(data.Config.Archs,
			basetypes.NewStringValue(arch.ToAPK()))
	}

	hash := sha256.Sum256([]byte(data.ConfigContents.ValueString()))
	data.Id = types.StringValue(hex.EncodeToString(hash[:]))

	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
