package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"chainguard.dev/apko/pkg/apk/apk"
	"chainguard.dev/apko/pkg/build"
	apkotypes "chainguard.dev/apko/pkg/build/types"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"k8s.io/apimachinery/pkg/util/sets"
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
				MarkdownDescription: "The package name to extract tags for. This matches a package in the config with this exact name, then a package whose name starts with `<target_package>-` (e.g. `foo` matches `foo-1.23`), and finally a package that declares `<target_package>` in its APK `provides` metadata (e.g. `kubectl` provided by `kubernetes-1.28`).",
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

	pkgs := map[string]string{}
	var found string
	for _, pkg := range ic.Contents.Packages {
		pkg, version, ok := strings.Cut(pkg, "=")
		if !ok {
			resp.Diagnostics.AddError("Invalid package", fmt.Sprintf("Invalid package: %s", pkg))
			return
		}
		pkgs[pkg] = version
	}
	if _, ok := pkgs[data.TargetPackage.ValueString()]; ok {
		found = data.TargetPackage.ValueString()
	} else {
		var foundver string
		for pkg, ver := range pkgs {
			// If the package name didn't match exactly, see if we have a package that starts with the target package name.
			// This is to handle the common case where a package named "foo" might be provided by a package named "foo-1.23".
			// In case there are multiple packages that match that provide different versions, we'll error out.
			if strings.HasPrefix(pkg, data.TargetPackage.ValueString()+"-") {
				if found != "" && foundver != ver {
					resp.Diagnostics.AddError("Multiple packages match", fmt.Sprintf("Multiple packages match with different versions: %s (%s) and %s (%s)", found, foundver, pkg, ver))
					return
				}

				found = pkg
				foundver = ver
				// Don't stop; keep looking in case there are multiple matches!
			}
		}
	}

	if found == "" {
		// Neither the exact name nor the prefix heuristic matched, so fall back
		// on the "provides" metadata of the locked packages. This handles e.g.
		// "kubectl" being provided by "kubernetes-1.28", or "foo-bar" being
		// provided by "foo-fips-bar".
		var diags diag.Diagnostics
		found, diags = d.findProvider(ctx, ic, pkgs, data.TargetPackage.ValueString())
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if found == "" {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to find package: %s...", data.TargetPackage.ValueString()), fmt.Sprintf("...in package list:\n\t%s", strings.Join(ic.Contents.Packages, "\n\t")))
		return
	}

	data.Tags = getStemmedVersionTags(pkgs[found])
	data.Tags = append(data.Tags, pkgs[found])
	sort.Strings(data.Tags)

	data.Id = types.StringValue(strings.Join(data.Tags, ","))

	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// findProvider resolves the locked packages in the image configuration to
// discover what each of them provides, since the locked package list itself
// does not carry this information, and returns the name of the package that
// provides target, or an empty string if no package does.
func (d *TagsDataSource) findProvider(ctx context.Context, ic apkotypes.ImageConfiguration, pkgs map[string]string, target string) (string, diag.Diagnostics) {
	_, ic2, err := fromImageData(ctx, ic, d.popts)
	if err != nil {
		return "", diag.Diagnostics{diag.NewErrorDiagnostic("Unable to parse apko config", err.Error())}
	}

	_, _, resolved, err := build.LockImageConfigurationWithPackages(ctx, *ic2,
		build.WithCache("", d.popts.planOffline, d.popts.cache),
		build.WithExtraKeys(d.popts.keyring),
		build.WithExtraBuildRepos(d.popts.buildRespositories),
		build.WithExtraRepos(d.popts.repositories),
		build.WithSizeLimits(toSizeLimits(d.popts.sizeLimits)),
		build.WithTransport(d.popts.transport))
	if err != nil {
		return "", diag.Diagnostics{diag.NewErrorDiagnostic(fmt.Sprintf("Unable to resolve packages providing %s", target), err.Error())}
	}

	found, err := providerOf(target, pkgs, resolved)
	if err != nil {
		return "", diag.Diagnostics{diag.NewErrorDiagnostic("Multiple packages match", err.Error())}
	}
	if found != "" {
		tflog.Debug(ctx, fmt.Sprintf("package %s is provided by %s=%s", target, found, pkgs[found]))
	}
	return found, nil
}

// providerOf returns the name of the package in pkgs (a map from package name
// to locked version) that declares target in its "provides" on any of the
// resolved architectures. It returns an empty string if no package provides
// target, and an error if multiple packages with different versions do.
func providerOf(target string, pkgs map[string]string, resolved map[apkotypes.Architecture][]*apk.RepositoryPackage) (string, error) {
	providers := sets.New[string]()
	for _, rpkgs := range resolved {
		for _, rpkg := range rpkgs {
			// Only consider the packages (and versions) that were locked in
			// the configuration we are extracting tags from.
			if ver, ok := pkgs[rpkg.Name]; !ok || ver != rpkg.Version {
				continue
			}
			for _, prov := range rpkg.Provides {
				// Provides entries take the form "name" or "name=version".
				name := prov
				if idx := strings.IndexAny(prov, "=<>~"); idx >= 0 {
					name = prov[:idx]
				}
				if name == target {
					providers.Insert(rpkg.Name)
					break
				}
			}
		}
	}

	var found string
	for _, pkg := range sets.List(providers) {
		if found != "" && pkgs[found] != pkgs[pkg] {
			return "", fmt.Errorf("multiple packages provide %s with different versions: %s (%s) and %s (%s)", target, found, pkgs[found], pkg, pkgs[pkg])
		}
		found = pkg
	}
	return found, nil
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
