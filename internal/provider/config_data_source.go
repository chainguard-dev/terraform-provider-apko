package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	apkotypes "chainguard.dev/apko/pkg/build/types"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/pkg/kmap"
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
	ExtraPackages      []string          `tfsdk:"extra_packages"`
	DefaultAnnotations map[string]string `tfsdk:"default_annotations"`
}

var imageConfigurationSchema basetypes.ObjectType

func init() {
	sch, err := generateType(apkotypes.ImageConfiguration{})
	if err != nil {
		panic(err)
	}
	imageConfigurationSchema = sch.(basetypes.ObjectType)
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
	if err := yaml.Unmarshal([]byte(data.ConfigContents.ValueString()), &ic); err != nil {
		resp.Diagnostics.AddError("Unable to parse apko configuration", err.Error())
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("got repos: %v", d.popts.repositories))
	tflog.Trace(ctx, fmt.Sprintf("got keyring: %v", d.popts.keyring))

	// Append any provider-specified repositories, packages, and keys, if specified.
	ic.Contents.Repositories = sets.List(sets.New(ic.Contents.Repositories...).Insert(d.popts.repositories...))
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

	// Resolve the package list to specific versions (as much as we can with
	// multi-arch), and overwrite the package list in the ImageConfiguration.
	pl, diags := d.resolvePackageList(ctx, ic)
	resp.Diagnostics = append(resp.Diagnostics, diags...)
	if diags.HasError() {
		return
	}
	ic.Contents.Packages = pl

	ov, diags := generateValue(ic)
	resp.Diagnostics = append(resp.Diagnostics, diags...)
	if diags.HasError() {
		return
	}
	data.Config = ov.(basetypes.ObjectValue)

	hash := sha256.Sum256([]byte(data.ConfigContents.ValueString()))
	data.Id = types.StringValue(hex.EncodeToString(hash[:]))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (d *ConfigDataSource) resolvePackageList(ctx context.Context, ic apkotypes.ImageConfiguration) ([]string, diag.Diagnostics) {
	workDir, err := os.MkdirTemp("", "apko-*")
	if err != nil {
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("Unable to create temp directory", err.Error())}
	}
	defer os.RemoveAll(workDir)

	eg := errgroup.Group{}
	archs := make([]resolved, len(ic.Archs))
	for i, arch := range ic.Archs {
		i, arch := i, arch
		eg.Go(func() error {
			bc, err := fromImageData(ic, d.popts, filepath.Join(workDir, arch.ToAPK()))
			if err != nil {
				return err
			}
			bc.Options.Arch = arch

			// Determine the exact versions of our transitive packages and lock them
			// down in the "resolved" configuration, so that this build may be
			// reproduced exactly.
			pkgs, _, err := bc.BuildPackageList(ctx)
			if err != nil {
				return err
			}
			r := resolved{
				// ParseArchitecture normalizes the architecture into the
				// canonical OCI form (amd64, not x86_64)
				arch:     apkotypes.ParseArchitecture(arch.ToAPK()).String(),
				packages: make(sets.Set[string], len(pkgs)),
				versions: make(map[string]string, len(pkgs)),
				provided: make(map[string]sets.Set[string], len(pkgs)),
			}
			for _, pkg := range pkgs {
				r.packages.Insert(pkg.Name)
				r.versions[pkg.Name] = pkg.Version

				for _, prov := range pkg.Provides {
					parts := packageNameRegex.FindAllStringSubmatch(prov, -1)
					if len(parts) == 0 || len(parts[0]) < 2 {
						continue
					}
					ps, ok := r.provided[pkg.Name]
					if !ok {
						ps = sets.New[string]()
					}
					ps.Insert(parts[0][1])
					r.provided[pkg.Name] = ps
				}
			}
			archs[i] = r
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("error computing package locks", err.Error())}
	}

	return unify(ic.Contents.Packages, archs)
}

type resolved struct {
	arch     string
	packages sets.Set[string]
	versions map[string]string
	provided map[string]sets.Set[string]
}

func unify(originals []string, inputs []resolved) ([]string, diag.Diagnostics) {
	if len(originals) == 0 {
		return nil, nil
	}
	originalPackages := resolved{
		packages: make(sets.Set[string], len(originals)),
		versions: make(map[string]string, len(originals)),
	}
	for _, orig := range originals {
		name := orig
		// The function we want from go-apk is private, but these are all the
		// special characters that delimit the package name from the cosntraint
		// so lop off the package name and stick the rest of the constraint into
		// the versions map.
		if idx := strings.IndexAny(orig, "=<>~"); idx >= 0 {
			name = orig[:idx]
		}
		originalPackages.packages.Insert(name)
		originalPackages.versions[name] = strings.TrimPrefix(orig, name)
	}

	// Start accumulating using the first entry, and unify it with the other
	// architectures.
	acc := resolved{
		packages: inputs[0].packages.Clone(),
		versions: kmap.Copy(inputs[0].versions),
		provided: inputs[0].provided,
	}
	for _, next := range inputs[1:] {
		if reflect.DeepEqual(acc.versions, next.versions) && reflect.DeepEqual(acc.provided, next.provided) {
			// If the package set's versions and provided packages match, then we're done.
			continue
		}

		// Remove any packages from our unification that do not appear in this
		// architecture's locked set.
		if diff := acc.packages.Difference(next.packages); diff.Len() > 0 {
			acc.packages.Delete(diff.UnsortedList()...)
		}
		// Walk through each of the packages remaining in our unification, and
		// remove any where this architecture disagrees with the unification.
		for _, pkg := range acc.packages.UnsortedList() {
			// When we find a package that has resolved differently, remove
			// it from our unified locked set.
			if acc.versions[pkg] != next.versions[pkg] {
				acc.packages.Delete(pkg)
				delete(acc.versions, pkg)
				delete(acc.provided, pkg)
			}
			if !acc.provided[pkg].Equal(next.provided[pkg]) {
				// If the package provides different things across architectures
				// then narrow what it provides to the common subset.
				acc.provided[pkg] = acc.provided[pkg].Intersection(next.provided[pkg])
			}
		}
	}

	var diagnostics diag.Diagnostics

	// Compute the set of original packages that are missing from our locked
	// configuration, and turn them into errors.
	missing := originalPackages.packages.Difference(acc.packages)
	if missing.Len() > 0 {
		for _, provider := range acc.provided {
			if provider == nil {
				// Doesn't provide anything
				continue
			}
			if provider.HasAny(missing.UnsortedList()...) {
				// This package provides some of the "missing" packages, so they
				// are not really missing.  Remove them from the "missing" set,
				// and elide the warning.
				missing = missing.Difference(provider)
			}
		}
		// There are still things missing even factoring in "provided" packages.
		if missing.Len() > 0 {
			for _, pkg := range sets.List(missing) {
				s := make(map[string]sets.Set[string], 2)
				for _, in := range inputs {
					set, ok := s[in.versions[pkg]]
					if !ok {
						set = sets.New[string]()
					}
					set.Insert(in.arch)
					s[in.versions[pkg]] = set
				}
				versionClusters := make([]string, 0, len(s))
				for k, v := range s {
					versionClusters = append(versionClusters, fmt.Sprintf("%s (%s)", k, strings.Join(sets.List(v), ", ")))
				}
				sort.Strings(versionClusters)
				// Append an error diagnostic with the packages we were unable to lock.
				diagnostics = append(diagnostics, diag.NewErrorDiagnostic(
					fmt.Sprintf("Unable to lock package %q to a consistent version", pkg),
					strings.Join(versionClusters, ", "),
				))
			}
		}
	}

	// Allocate a list sufficient for holding all of our locked package versions
	// as well as the packages we were unable to lock.
	pl := make([]string, 0, len(acc.versions)+missing.Len())

	// Append any missing packages with their original constraints coming in.
	// NOTE: the originalPackages "versions" includes the remainder of the
	// package constraint including the operator.
	for _, pkg := range sets.List(missing) {
		if ver := originalPackages.versions[pkg]; ver != "" {
			pl = append(pl, fmt.Sprintf("%s%s", pkg, ver))
		} else {
			pl = append(pl, pkg)
		}
	}

	// Append all of the resolved and unified packages with an exact match
	// based on the resolved version we found.
	for _, pkg := range sets.List(acc.packages) {
		pl = append(pl, fmt.Sprintf("%s=%s", pkg, acc.versions[pkg]))
	}

	// If a particular architecture is missing additional packages from the
	// locked set that it produced, than warn about those as well.
	for _, input := range inputs {
		missingHere := input.packages.Difference(acc.packages).Difference(missing)
		if missingHere.Len() > 0 {
			diagnostics = append(diagnostics, diag.NewWarningDiagnostic(
				fmt.Sprintf("unable to lock certain packages for %s", input.arch),
				fmt.Sprint(sets.List(missingHere)),
			))
		}
	}

	return pl, diagnostics
}

// Copied from go-apk's version.go
var packageNameRegex = regexp.MustCompile(`^([^@=><~]+)(([=><~]+)([^@]+))?(@([a-zA-Z0-9]+))?$`)
