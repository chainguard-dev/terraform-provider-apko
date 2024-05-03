package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	apkotypes "chainguard.dev/apko/pkg/build/types"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &TagsDataSource{}

func NewTagsDataSource() datasource.DataSource {
	return &TagsDataSource{}
}

// TagsDataSource defines the data source implementation.
type TagsDataSource struct {
	popts ProviderOpts
}

// TagsDataSourceModel describes the data source data model.
type TagsDataSourceModel struct {
	Id            types.String `tfsdk:"id"`
	Config        types.Object `tfsdk:"config"`
	TargetPackage types.String `tfsdk:"target_package"`

	Tags []string `tfsdk:"tags"`
}

func (d *TagsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tags"
}

func (d *TagsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This reads an apko configuration file into a structured form.",
		Attributes: map[string]schema.Attribute{
			"config": schema.ObjectAttribute{
				MarkdownDescription: "The parsed structure of the apko configuration.",
				Required:            true,
				AttributeTypes:      imageConfigurationSchema.AttrTypes,
			},
			"target_package": schema.StringAttribute{
				MarkdownDescription: "The package name to extract tags for.",
				Required:            true,
			},
			"tags": schema.ListAttribute{
				MarkdownDescription: "The tags for the target package.",
				Computed:            true,
				ElementType:         basetypes.StringType{},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for this apko config.",
				Computed:            true,
			},
		},
	}
}

func (d *TagsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *TagsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data TagsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, set := os.LookupEnv("TF_APKO_DISABLE_VERSION_TAGS"); set {
		resp.Diagnostics.AddWarning("Version tags disabled", "Version tags are disabled using TF_APKO_DISABLE_VERSION_TAGS environment variable")
		data.Tags = []string{}
		data.Id = types.StringValue("disabled")
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	var ic apkotypes.ImageConfiguration
	if diags := assignValue(data.Config, &ic); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	found := false
	for _, pkg := range ic.Contents.Packages {
		if strings.HasPrefix(pkg, data.TargetPackage.ValueString()+"=") {
			fullVersion := strings.TrimPrefix(pkg, data.TargetPackage.ValueString()+"=")
			data.Tags = getStemmedVersionTags(fullVersion)
			data.Tags = append(data.Tags, fullVersion)
			sort.Strings(data.Tags)
			found = true
			break
		}
	}
	if !found {
        resp.Diagnostics.AddError(fmt.Sprintf("Unable to find package: %s...", data.TargetPackage.ValueString()), fmt.Sprintf("...in package list:\n\t%s", strings.Join(ic.Contents.Packages, "\n\t")))
		return
	}

	data.Id = types.StringValue(strings.Join(data.Tags, ","))

	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Copied from https://github.com/chainguard-dev/apko/blob/894dcbee4f44709e5702be03d19a581aeadb5941/pkg/apk/apk.go#L197
// TODO: use version parser from https://gitlab.alpinelinux.org/alpine/go/-/tree/master/version
func getStemmedVersionTags(version string) []string {
	tags := []string{}
	re := regexp.MustCompile("[.]+")
	tmp := []string{}
	for _, part := range re.Split(version, -1) {
		tmp = append(tmp, part)
		additionalTag := strings.Join(tmp, ".")
		if additionalTag == version {
			tmp := strings.Split(version, "-")
			additionalTag = strings.Join(tmp[:len(tmp)-1], "-")
		}
		tags = append(tags, additionalTag)
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[j] < tags[i]
	})
	return tags
}
