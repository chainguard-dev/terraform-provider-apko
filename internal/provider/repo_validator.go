package provider

import (
	"context"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type repoValidator struct{}

var _ validator.String = repoValidator{}

func (v repoValidator) Description(context.Context) string {
	return "value must be a valid OCI repository"
}
func (v repoValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v repoValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	if _, err := name.NewRepository(val); err != nil {
		resp.Diagnostics.AddError("Invalid OCI repository", err.Error())
	}
}
