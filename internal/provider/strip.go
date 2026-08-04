package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// stripAttrType removes the attribute at the given path from the object type.
func stripAttrType(t basetypes.ObjectType, path []string) basetypes.ObjectType {
	if len(path) == 1 {
		delete(t.AttrTypes, path[0])
		return t
	}
	child, ok := t.AttrTypes[path[0]].(basetypes.ObjectType)
	if !ok {
		panic(fmt.Sprintf("expected %q to be an object type", path[0]))
	}
	t.AttrTypes[path[0]] = stripAttrType(child, path[1:])
	return t
}

// stripAttrs removes the attribute at the given path from the attribute map,
// rebuilding nested object values against the given (already stripped) type
// as needed. The caller rebuilds the top-level object once all paths are
// stripped.
func stripAttrs(attrs map[string]attr.Value, t basetypes.ObjectType, path []string) diag.Diagnostics {
	if len(path) == 1 {
		delete(attrs, path[0])
		return nil
	}
	childVal, ok := attrs[path[0]].(basetypes.ObjectValue)
	if !ok {
		return nil
	}
	childType, ok := t.AttrTypes[path[0]].(basetypes.ObjectType)
	if !ok {
		return nil
	}
	childAttrs := childVal.Attributes()
	if diags := stripAttrs(childAttrs, childType, path[1:]); diags.HasError() {
		return diags
	}
	newChild, diags := types.ObjectValue(childType.AttrTypes, childAttrs)
	if diags.HasError() {
		return diags
	}
	attrs[path[0]] = newChild
	return diags
}
