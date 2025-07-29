package provider

import (
	"context"
	"runtime/debug"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ function.Function = &VersionFunction{}

func NewVersionFunction(providerVersion string) function.Function {
	return &VersionFunction{
		providerVersion: providerVersion,
	}
}

// VersionFunction defines the function implementation.
type VersionFunction struct {
	providerVersion string
}

func (f *VersionFunction) Metadata(ctx context.Context, req function.MetadataRequest, resp *function.MetadataResponse) {
	resp.Name = "version"
}

func (f *VersionFunction) Definition(ctx context.Context, req function.DefinitionRequest, resp *function.DefinitionResponse) {
	resp.Definition = function.Definition{
		Summary:             "Get version information",
		MarkdownDescription: "Returns version information for the terraform-provider-apko and the underlying apko package.",
		Return: function.ObjectReturn{
			AttributeTypes: map[string]attr.Type{
				"provider_version": types.StringType,
				"apko_version":     types.StringType,
			},
		},
	}
}

func (f *VersionFunction) Run(ctx context.Context, req function.RunRequest, resp *function.RunResponse) {
	// Use the provider version stored in the function
	providerVersion := f.providerVersion
	if providerVersion == "" {
		providerVersion = "unknown"
	}

	// Get the apko version from build info
	apkoVersion := "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "chainguard.dev/apko" {
				apkoVersion = dep.Version
				break
			}
		}
	}

	// Create the return object
	objectValue, diags := types.ObjectValue(
		map[string]attr.Type{
			"provider_version": types.StringType,
			"apko_version":     types.StringType,
		},
		map[string]attr.Value{
			"provider_version": types.StringValue(providerVersion),
			"apko_version":     types.StringValue(apkoVersion),
		},
	)

	if diags.HasError() {
		resp.Error = function.FuncErrorFromDiags(ctx, diags)
		return
	}

	resp.Error = function.ConcatFuncErrors(resp.Error, resp.Result.Set(ctx, objectValue))
}
