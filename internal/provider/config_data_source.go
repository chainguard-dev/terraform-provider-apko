package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"chainguard.dev/apko/pkg/apk/apk"
	"chainguard.dev/apko/pkg/build"
	apkotypes "chainguard.dev/apko/pkg/build/types"
	"chainguard.dev/apko/pkg/sbom/generator/spdx"
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
	ResolvedPackages   types.Map         `tfsdk:"resolved_packages"`
}

var imageConfigurationSchema basetypes.ObjectType
var imageConfigurationsSchema basetypes.ObjectType

// resolvedPackageSchema describes a single resolved APK package: the URL of the
// .apk to download plus the metadata carried in the APKINDEX entry.
var resolvedPackageSchema = basetypes.ObjectType{
	AttrTypes: map[string]attr.Type{
		"name":              basetypes.StringType{},
		"version":           basetypes.StringType{},
		"arch":              basetypes.StringType{},
		"url":               basetypes.StringType{},
		"checksum":          basetypes.StringType{},
		"origin":            basetypes.StringType{},
		"description":       basetypes.StringType{},
		"license":           basetypes.StringType{},
		"maintainer":        basetypes.StringType{},
		"homepage":          basetypes.StringType{},
		"repo_commit":       basetypes.StringType{},
		"build_date":        basetypes.Int64Type{},
		"size":              basetypes.Int64Type{},
		"installed_size":    basetypes.Int64Type{},
		"provider_priority": basetypes.Int64Type{},
		"provides":          basetypes.ListType{ElemType: basetypes.StringType{}},
		"dependencies":      basetypes.ListType{ElemType: basetypes.StringType{}},
	},
}

// stringListValue converts a []string into a Terraform list of strings.
func stringListValue(ss []string) (basetypes.ListValue, diag.Diagnostics) {
	vals := make([]attr.Value, len(ss))
	for i, s := range ss {
		vals[i] = types.StringValue(s)
	}
	return types.ListValue(basetypes.StringType{}, vals)
}

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

	// TODO: Certificates are optional, but our schema generation using
	// schema.ObjectAttribute with AttributeTypes doesn't support field-level
	// optional/required controls. For now, remove certificates from the schema
	// type definition. We also remove it from generated values in the Read method.
	delete(imageConfigurationSchema.AttrTypes, "certificates")

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
			"resolved_packages": schema.MapAttribute{
				MarkdownDescription: "A map from the APK architecture to the list of resolved packages for that architecture. " +
					"Each entry carries the `url` of the `.apk` to download plus the metadata from the package's APKINDEX entry: " +
					"`name`, `version`, `arch`, `checksum` (the APKv2 package checksum, `Q1` + base64-encoded SHA1 of the control " +
					"section, see the [APKv2 package checksum field](https://wiki.alpinelinux.org/wiki/Apk_spec#Package_Checksum_Field)), " +
					"`origin`, `description`, `license`, `maintainer`, `homepage`, `repo_commit`, `build_date` (Unix seconds), " +
					"`size`, `installed_size`, `provider_priority`, `provides`, and `dependencies`. " +
					"The special `index` key contains the deduplicated union across all architectures.",
				Computed: true,
				ElementType: basetypes.ListType{
					ElemType: resolvedPackageSchema,
				},
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
	ic.Contents.Repositories = sets.List(sets.New(ic.Contents.Repositories...).Insert(d.popts.repositories...))
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

	// Apply provider-level layering configuration if none is specified in the image config
	if ic.Layering == nil && d.popts.layering != nil {
		// No layering specified in config, apply provider defaults
		ic.Layering = &apkotypes.Layering{
			Strategy: d.popts.layering.Strategy,
			Budget:   d.popts.layering.Budget,
		}
	}
	// When layering:{} is present, we preserve the empty object as-is

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
	pls, resolvedPkgs, diags := d.resolvePackageList(ctx, ic)
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

		// Remove certificates from the generated value to match the schema
		// TODO: see above about optional types.
		attrs := cfg.Attributes()
		delete(attrs, "certificates")
		cfg, diags = types.ObjectValue(imageConfigurationSchema.AttrTypes, attrs)
		resp.Diagnostics = append(resp.Diagnostics, diags...)
		if diags.HasError() {
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

	// Surface the resolved packages (download URL + APKINDEX metadata) per
	// architecture, so consumers can populate e.g. SLSA provenance
	// resolvedDependencies. The special "index" key holds the deduplicated
	// union across all architectures.
	resolvedPackages, diags := resolvedPackagesMap(resolvedPkgs)
	resp.Diagnostics = append(resp.Diagnostics, diags...)
	if diags.HasError() {
		return
	}
	data.ResolvedPackages = resolvedPackages

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

// resolvedPackagesMap converts the per-architecture resolved packages into the
// Terraform value for the "resolved_packages" attribute: a map from the
// canonical architecture to its list of packages, plus a deduplicated "index"
// union across all architectures. Architectures are processed in sorted order
// so the output (in particular the "index" union) is deterministic.
func resolvedPackagesMap(resolvedPkgs map[apkotypes.Architecture][]*apk.RepositoryPackage) (basetypes.MapValue, diag.Diagnostics) {
	resolvedListType := basetypes.ListType{ElemType: resolvedPackageSchema}
	var diagnostics diag.Diagnostics

	// Normalize to the same canonical arch keys used for "configs", and sort
	// them so iteration (and the resulting "index" union) is deterministic.
	byArch := make(map[string][]*apk.RepositoryPackage, len(resolvedPkgs))
	archKeys := make([]string, 0, len(resolvedPkgs))
	for arch, pkgs := range resolvedPkgs {
		archKey := apkotypes.ParseArchitecture(arch.ToAPK()).String()
		byArch[archKey] = pkgs
		archKeys = append(archKeys, archKey)
	}
	sort.Strings(archKeys)

	rpMap := make(map[string]attr.Value, len(byArch)+1)
	indexSeen := make(map[string]struct{})
	indexList := make([]attr.Value, 0)

	for _, archKey := range archKeys {
		pkgs := byArch[archKey]
		list := make([]attr.Value, 0, len(pkgs))
		for _, rp := range pkgs {
			obj, diags := resolvedPackageObject(rp)
			diagnostics = append(diagnostics, diags...)
			if diags.HasError() {
				return basetypes.MapValue{}, diagnostics
			}
			list = append(list, obj)

			if _, ok := indexSeen[rp.URL()]; !ok {
				indexSeen[rp.URL()] = struct{}{}
				indexList = append(indexList, obj)
			}
		}

		lv, diags := types.ListValue(resolvedPackageSchema, list)
		diagnostics = append(diagnostics, diags...)
		if diags.HasError() {
			return basetypes.MapValue{}, diagnostics
		}
		rpMap[archKey] = lv
	}

	indexListValue, diags := types.ListValue(resolvedPackageSchema, indexList)
	diagnostics = append(diagnostics, diags...)
	if diags.HasError() {
		return basetypes.MapValue{}, diagnostics
	}
	rpMap["index"] = indexListValue

	m, diags := types.MapValue(resolvedListType, rpMap)
	diagnostics = append(diagnostics, diags...)
	return m, diagnostics
}

// resolvedPackageObject builds the Terraform object for a single resolved
// package from its APKINDEX metadata.
func resolvedPackageObject(rp *apk.RepositoryPackage) (basetypes.ObjectValue, diag.Diagnostics) {
	var diagnostics diag.Diagnostics
	provides, diags := stringListValue(rp.Provides)
	diagnostics = append(diagnostics, diags...)
	deps, diags := stringListValue(rp.Dependencies)
	diagnostics = append(diagnostics, diags...)
	if diagnostics.HasError() {
		return basetypes.ObjectValue{}, diagnostics
	}

	obj, diags := types.ObjectValue(resolvedPackageSchema.AttrTypes, map[string]attr.Value{
		"name":     types.StringValue(rp.Name),
		"version":  types.StringValue(rp.Version),
		"arch":     types.StringValue(rp.Arch),
		"url":      types.StringValue(rp.URL()),
		"checksum": types.StringValue(rp.ChecksumString()),
		"origin":   types.StringValue(rp.Origin),
		// rp.URL() is the download URL (method); rp.Package.URL is the upstream
		// homepage from the APKINDEX "U" field.
		"description":       types.StringValue(rp.Description),
		"license":           types.StringValue(rp.License),
		"maintainer":        types.StringValue(rp.Maintainer),
		"homepage":          types.StringValue(rp.Package.URL),
		"repo_commit":       types.StringValue(rp.RepoCommit),
		"build_date":        types.Int64Value(rp.BuildDate),
		"size":              types.Int64Value(int64(rp.Size)),
		"installed_size":    types.Int64Value(int64(rp.InstalledSize)),
		"provider_priority": types.Int64Value(int64(rp.ProviderPriority)),
		"provides":          provides,
		"dependencies":      deps,
	})
	diagnostics = append(diagnostics, diags...)
	return obj, diagnostics
}

func (d *ConfigDataSource) resolvePackageList(ctx context.Context, ic apkotypes.ImageConfiguration) (map[string]*apkotypes.ImageConfiguration, map[apkotypes.Architecture][]*apk.RepositoryPackage, diag.Diagnostics) {
	_, ic2, err := fromImageData(ctx, ic, d.popts)
	if err != nil {
		return nil, nil, diag.Diagnostics{diag.NewErrorDiagnostic("Unable to parse apko config", err.Error())}
	}

	pls, missingByArch, resolvedPkgs, err := build.LockImageConfigurationWithPackages(ctx, *ic2,
		build.WithCache("", d.popts.planOffline, d.popts.cache),
		build.WithSBOMGenerators(spdx.New()),
		build.WithExtraKeys(d.popts.keyring),
		build.WithExtraBuildRepos(d.popts.buildRespositories),
		build.WithExtraRepos(d.popts.repositories))
	if err != nil {
		// These are a nightmare to debug, so we're going to try to include the apko config in the error.
		b, merr := json.MarshalIndent(ic, "", "  ")
		if merr != nil {
			// If we can't marshal the config, just return the original error.
			return nil, nil, diag.Diagnostics{diag.NewErrorDiagnostic("computing package locks", err.Error())}
		}

		// Otherwise include both the config and the error in the details.
		details := fmt.Sprintf("apko config:\n%s\n\nerror:\n%s", string(b), err)
		return nil, nil, diag.Diagnostics{diag.NewErrorDiagnostic("computing package locks", details)}
	}

	var diagnostics diag.Diagnostics

	for arch, missing := range missingByArch {
		diagnostics = append(diagnostics, diag.NewWarningDiagnostic(
			fmt.Sprintf("unable to lock certain packages for %s", arch),
			fmt.Sprint(missing),
		))
	}

	return pls, resolvedPkgs, diagnostics
}
