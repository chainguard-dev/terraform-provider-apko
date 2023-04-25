package provider

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &BuildResource{}
var _ resource.ResourceWithImportState = &BuildResource{}

func NewBuildResource() resource.Resource {
	return &BuildResource{}
}

type BuildResource struct {
}

type BuildResourceModel struct {
	Id       types.String `tfsdk:"id"`
	Repo     types.String `tfsdk:"repo"`
	Config   types.String `tfsdk:"config"`
	ImageRef types.String `tfsdk:"image_ref"`
}

func (r *BuildResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_build"
}

func (r *BuildResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This performs an apko build from the provided config file",
		Attributes: map[string]schema.Attribute{
			"repo": schema.StringAttribute{
				MarkdownDescription: "The name of the container repository to which we should publish the image.",
				Required:            true,
				Validators:          []validator.String{repoValidator{}},
			},
			"config": schema.StringAttribute{
				MarkdownDescription: "The apko configuration file.",
				Required:            true,
				// TODO: validate the apko config.
			},
			"image_ref": schema.StringAttribute{
				MarkdownDescription: "The resulting fully-qualified digest (e.g. {repo}@sha256:deadbeef).",
				Computed:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The resulting fully-qualified digest (e.g. {repo}@sha256:deadbeef).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *BuildResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *BuildResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repo, err := name.NewRepository(data.Repo.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Error parsing repo: %v", err))
		return
	}

	digest, _, err := doBuild(ctx, *data)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}
	dig := repo.Digest(digest.String()).String()

	data.Id = types.StringValue(dig)
	data.ImageRef = types.StringValue(dig)

	tflog.Trace(ctx, "created a resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BuildResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *BuildResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repo, err := name.NewRepository(data.Repo.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Error parsing repo: %v", err))
		return
	}

	digest, _, err := doBuild(ctx, *data)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}
	dig := repo.Digest(digest.String()).String()

	if dig != data.ImageRef.ValueString() {
		data.Id = types.StringValue("")
		data.ImageRef = types.StringValue("")
	} else {
		data.Id = types.StringValue(dig)
		data.ImageRef = types.StringValue(dig)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BuildResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *BuildResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repo, err := name.NewRepository(data.Repo.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Error parsing repo: %v", err))
		return
	}

	digest, _, err := doBuild(ctx, *data)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}
	dig := repo.Digest(digest.String()).String()

	data.Id = types.StringValue(dig)
	data.ImageRef = types.StringValue(dig)

	tflog.Trace(ctx, "updated a resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BuildResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *BuildResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// TODO: If we ever want to delete the image from the registry, we can do it here.
}

func (r *BuildResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
