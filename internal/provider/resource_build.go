package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/chainguard-dev/terraform-provider-oci/pkg/validators"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &BuildResource{}
var _ resource.ResourceWithImportState = &BuildResource{}

func NewBuildResource() resource.Resource {
	return &BuildResource{}
}

type BuildResource struct {
	popts ProviderOpts
}

type BuildResourceModel struct {
	Id       types.String `tfsdk:"id"`
	Repo     types.String `tfsdk:"repo"`
	Config   types.Object `tfsdk:"config"`
	ImageRef types.String `tfsdk:"image_ref"`

	SBOMs types.Map `tfsdk:"sboms"`

	popts ProviderOpts // Data passed from the provider.
}

var digestSBOMSchema = basetypes.ObjectType{
	AttrTypes: map[string]attr.Type{
		"digest":         basetypes.StringType{},
		"predicate_type": basetypes.StringType{},
		// TODO(mattmoor): is there a way to designate this path's value as
		// unimportant for the purposes of planning?
		"predicate_path":   basetypes.StringType{},
		"predicate_sha256": basetypes.StringType{},
	},
}

func (r *BuildResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_build"
}

func (r *BuildResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	popts, ok := req.ProviderData.(*ProviderOpts)
	if !ok || popts == nil {
		resp.Diagnostics.AddError("Client Error", "invalid provider data")
		return
	}
	r.popts = *popts
}

func (r *BuildResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This performs an apko build from the provided config file",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The resulting fully-qualified digest (e.g. {repo}@sha256:deadbeef).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"repo": schema.StringAttribute{
				MarkdownDescription: "The name of the container repository to which we should publish the image.",
				Required:            true,
				Validators:          []validator.String{validators.RepoValidator{}},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"config": schema.ObjectAttribute{
				MarkdownDescription: "The parsed structure of the apko configuration.",
				Required:            true,
				AttributeTypes:      imageConfigurationSchema.AttrTypes,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
			},
			"image_ref": schema.StringAttribute{
				MarkdownDescription: "The resulting fully-qualified digest (e.g. {repo}@sha256:deadbeef).",
				Computed:            true,
			},
			"sboms": schema.MapNestedAttribute{
				MarkdownDescription: "A map from the APK architecture to the digest for that architecture and its SBOM.",
				Computed:            true,
				Optional:            true,
				Required:            false,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"digest": schema.StringAttribute{
							MarkdownDescription: "The digest of the index or image.",
							Computed:            true,
							Optional:            true,
							Required:            false,
						},
						"predicate_type": schema.StringAttribute{
							MarkdownDescription: "The predicate type of the SBOM.",
							Computed:            true,
							Optional:            true,
							Required:            false,
						},
						"predicate_path": schema.StringAttribute{
							MarkdownDescription: "The path to the SBOM contents.",
							Computed:            true,
							Optional:            true,
							Required:            false,
						},
						"predicate_sha256": schema.StringAttribute{
							MarkdownDescription: "The hex-encoded SHA256 hash of the SBOM contents.",
							Computed:            true,
							Optional:            true,
							Required:            false,
						},
					},
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
	data.popts = r.popts

	repo, err := name.NewRepository(data.Repo.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Error parsing repo: %v", err))
		return
	}

	tempDir, err := os.MkdirTemp("", "apko-*")
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Errorf("failed to create temporary directory: %w", err).Error())
		return
	}
	defer os.RemoveAll(tempDir)

	digest, se, sboms, err := doBuild(ctx, *data, tempDir)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}
	dig := repo.Digest(digest.String())

	pushable, ok := se.(remote.Taggable)
	if !ok {
		resp.Diagnostics.AddError("unexpected type", dig.String())
		return
	}

	pusher, err := remote.NewPusher(r.popts.ropts...)
	if err != nil {
		resp.Diagnostics.AddError("NewPusher", err.Error())
		return
	}
	if err := retry(ctx, longBackoff, func(ctx context.Context) error {
		return pusher.Push(ctx, dig, pushable)
	}); err != nil {
		resp.Diagnostics.AddError("Error publishing "+dig.String(), err.Error())
		return
	}

	data.Id = types.StringValue(dig.String())
	data.ImageRef = types.StringValue(dig.String())

	sbv := make(map[string]attr.Value, len(sboms))
	for k, v := range sboms {
		val, diags := types.ObjectValue(digestSBOMSchema.AttrTypes, map[string]attr.Value{
			"digest":           types.StringValue(repo.Digest(v.imageHash.String()).String()),
			"predicate_type":   types.StringValue(v.predicateType),
			"predicate_path":   types.StringValue(v.predicatePath),
			"predicate_sha256": types.StringValue(v.predicateSHA256),
		})
		resp.Diagnostics = append(resp.Diagnostics, diags...)
		if diags.HasError() {
			return
		}
		sbv[k] = val
	}
	sv, diags := types.MapValue(digestSBOMSchema, sbv)
	if diags != nil {
		resp.Diagnostics = append(resp.Diagnostics, diags...)
		return
	}
	data.SBOMs = sv

	tflog.Trace(ctx, "created a resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BuildResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *BuildResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.popts = r.popts

	// We "lock" the config and changes to it already require replacement.

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BuildResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *BuildResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.popts = r.popts

	repo, err := name.NewRepository(data.Repo.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Error parsing repo: %v", err))
		return
	}

	tempDir, err := os.MkdirTemp("", "apko-*")
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Errorf("failed to create temporary directory: %w", err).Error())
		return
	}
	defer os.RemoveAll(tempDir)

	digest, _, _, err := doBuild(ctx, *data, tempDir)
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
