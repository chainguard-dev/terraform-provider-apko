package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type RFC3339Validator struct{}

var _ validator.String = RFC3339Validator{}

func (v RFC3339Validator) Description(context.Context) string {
	return "value must be a valid RFC3339-encoded timestamp"
}
func (v RFC3339Validator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v RFC3339Validator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	if _, err := time.Parse(time.RFC3339, val); err != nil {
		resp.Diagnostics.AddError("Invalid RFC3339-encoded timestamp", err.Error())
	}
}
