package provider

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

func contentsAttrTypes(t *testing.T, obj basetypes.ObjectType) map[string]attr.Type {
	t.Helper()
	contents, ok := obj.AttrTypes["contents"].(basetypes.ObjectType)
	if !ok {
		t.Fatalf("expected contents to be an object type, got %T", obj.AttrTypes["contents"])
	}
	return contents.AttrTypes
}

func testObjectType() basetypes.ObjectType {
	return basetypes.ObjectType{
		AttrTypes: map[string]attr.Type{
			"certificates": basetypes.StringType{},
			"cmd":          basetypes.StringType{},
			"contents": basetypes.ObjectType{
				AttrTypes: map[string]attr.Type{
					"packages":        basetypes.ListType{ElemType: basetypes.StringType{}},
					"runtime_keyring": basetypes.ListType{ElemType: basetypes.StringType{}},
				},
			},
		},
	}
}

func TestStripAttrType(t *testing.T) {
	tests := []struct {
		name string
		path []string
		want basetypes.ObjectType
	}{{
		name: "top-level attribute",
		path: []string{"certificates"},
		want: basetypes.ObjectType{
			AttrTypes: map[string]attr.Type{
				"cmd": basetypes.StringType{},
				"contents": basetypes.ObjectType{
					AttrTypes: map[string]attr.Type{
						"packages":        basetypes.ListType{ElemType: basetypes.StringType{}},
						"runtime_keyring": basetypes.ListType{ElemType: basetypes.StringType{}},
					},
				},
			},
		},
	}, {
		name: "nested attribute",
		path: []string{"contents", "runtime_keyring"},
		want: basetypes.ObjectType{
			AttrTypes: map[string]attr.Type{
				"certificates": basetypes.StringType{},
				"cmd":          basetypes.StringType{},
				"contents": basetypes.ObjectType{
					AttrTypes: map[string]attr.Type{
						"packages": basetypes.ListType{ElemType: basetypes.StringType{}},
					},
				},
			},
		},
	}, {
		name: "missing attribute is a no-op",
		path: []string{"does_not_exist"},
		want: basetypes.ObjectType{
			AttrTypes: map[string]attr.Type{
				"certificates": basetypes.StringType{},
				"cmd":          basetypes.StringType{},
				"contents": basetypes.ObjectType{
					AttrTypes: map[string]attr.Type{
						"packages":        basetypes.ListType{ElemType: basetypes.StringType{}},
						"runtime_keyring": basetypes.ListType{ElemType: basetypes.StringType{}},
					},
				},
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := stripAttrType(testObjectType(), test.path)
			if diff := cmp.Diff(got, test.want); diff != "" {
				t.Errorf("stripAttrType(%v) mismatch (-got, +want):\n%s", test.path, diff)
			}
		})
	}
}

func TestStripAttrTypePanicsOnNonObject(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic when traversing through a non-object attribute")
		}
	}()
	stripAttrType(testObjectType(), []string{"cmd", "nested"})
}

func TestStripAttrs(t *testing.T) {
	// The stripped type, i.e. what values must conform to after stripping.
	strippedType := testObjectType()
	for _, path := range [][]string{{"certificates"}, {"contents", "runtime_keyring"}} {
		strippedType = stripAttrType(strippedType, path)
	}

	packages, diags := types.ListValue(basetypes.StringType{}, []attr.Value{types.StringValue("foo=1.2.3")})
	if diags.HasError() {
		t.Fatalf("ListValue: %v", diags)
	}
	keyring, diags := types.ListValue(basetypes.StringType{}, []attr.Value{types.StringValue("key")})
	if diags.HasError() {
		t.Fatalf("ListValue: %v", diags)
	}
	contents, diags := types.ObjectValue(contentsAttrTypes(t, testObjectType()), map[string]attr.Value{
		"packages":        packages,
		"runtime_keyring": keyring,
	})
	if diags.HasError() {
		t.Fatalf("ObjectValue: %v", diags)
	}

	attrs := map[string]attr.Value{
		"certificates": types.StringValue("cert"),
		"cmd":          types.StringValue("/bin/sh"),
		"contents":     contents,
	}

	// Strip all paths first, then rebuild the top-level object once, exactly
	// like the Read method does. Rebuilding per path would fail because the
	// value would still hold attributes pending removal by a later path.
	for _, path := range [][]string{{"certificates"}, {"contents", "runtime_keyring"}} {
		if diags := stripAttrs(attrs, strippedType, path); diags.HasError() {
			t.Fatalf("stripAttrs(%v): %v", path, diags)
		}
	}

	got, diags := types.ObjectValue(strippedType.AttrTypes, attrs)
	if diags.HasError() {
		t.Fatalf("ObjectValue after strip: %v", diags)
	}

	wantContents, diags := types.ObjectValue(contentsAttrTypes(t, strippedType), map[string]attr.Value{
		"packages": packages,
	})
	if diags.HasError() {
		t.Fatalf("ObjectValue: %v", diags)
	}
	want, diags := types.ObjectValue(strippedType.AttrTypes, map[string]attr.Value{
		"cmd":      types.StringValue("/bin/sh"),
		"contents": wantContents,
	})
	if diags.HasError() {
		t.Fatalf("ObjectValue: %v", diags)
	}

	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("stripped value mismatch (-got, +want):\n%s", diff)
	}
}

func TestStripAttrsMissingIsNoop(t *testing.T) {
	attrs := map[string]attr.Value{
		"cmd": types.StringValue("/bin/sh"),
	}
	// Neither the attribute nor the nested parent exist; both must be no-ops.
	if diags := stripAttrs(attrs, testObjectType(), []string{"does_not_exist"}); diags.HasError() {
		t.Fatalf("stripAttrs: %v", diags)
	}
	if diags := stripAttrs(attrs, testObjectType(), []string{"missing_parent", "child"}); diags.HasError() {
		t.Fatalf("stripAttrs: %v", diags)
	}
	if len(attrs) != 1 {
		t.Errorf("expected attrs to be unchanged, got %v", attrs)
	}
}
