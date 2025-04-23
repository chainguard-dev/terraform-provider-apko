package provider

import (
	"testing"

	"chainguard.dev/apko/pkg/build/types"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"gopkg.in/yaml.v2"
)

func TestGenerateType(t *testing.T) {
	tests := []struct {
		name string
		obj  interface{}
		want attr.Type
	}{{
		name: "string",
		obj:  "",
		want: basetypes.StringType{},
	}, {
		name: "int64",
		obj:  int64(3),
		want: basetypes.Int64Type{},
	}, {
		name: "float64",
		obj:  float64(3.1415926),
		want: basetypes.Float64Type{},
	}, {
		name: "bool",
		obj:  true,
		want: basetypes.BoolType{},
	}, {
		name: "string slice",
		obj:  []string{},
		want: basetypes.ListType{
			ElemType: basetypes.StringType{},
		},
	}, {
		name: "int64 slice",
		obj:  []int64{},
		want: basetypes.ListType{
			ElemType: basetypes.Int64Type{},
		},
	}, {
		name: "map string to string",
		obj:  map[string]string{},
		want: basetypes.MapType{
			ElemType: basetypes.StringType{},
		},
	}, {
		name: "map string to int64",
		obj:  map[string]int64{},
		want: basetypes.MapType{
			ElemType: basetypes.Int64Type{},
		},
	}, {
		name: "object",
		obj: struct {
			Name   string
			Tagged int64 `yaml:"different"`
			Nested struct {
				Foo     bool   `yaml:"blah"`
				Skipped string `yaml:"-"`
				Bar     []string
			}
		}{},
		want: basetypes.ObjectType{
			AttrTypes: map[string]attr.Type{
				"name":      basetypes.StringType{},
				"different": basetypes.Int64Type{},
				"nested": basetypes.ObjectType{
					AttrTypes: map[string]attr.Type{
						"blah": basetypes.BoolType{},
						"bar": basetypes.ListType{
							ElemType: basetypes.StringType{},
						},
					},
				},
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := generateType(test.obj)
			if err != nil {
				t.Fatalf("generateType() = %v", err)
			}
			if !test.want.Equal(got) {
				t.Errorf("got = %+v, wanted %v", got, test.want)
			}
		})
	}
}
func TestRoundtripValue(t *testing.T) {
	// Test that we roundtrip pointers properly.
	type Bar struct {
		Field string
	}

	type Foo struct {
		Bar *Bar
	}

	content := `
contents:
  repositories:
  - https://packages.wolfi.dev/os
  keyring:
  - https://packages.wolfi.dev/os/wolfi-signing.rsa.pub
  packages:
  - wolfi-baselayout
  - ca-certificates-bundle
  - tzdata

accounts:
  groups:
  - groupname: nonroot
    gid: 65532
  users:
  - username: nonroot
    uid: 65532
    gid: 65532
  run-as: 65532


environment:
  HOME: /tmp

archs:
- x86_64
- aarch64
`
	var want types.ImageConfiguration
	if err := yaml.Unmarshal([]byte(content), &want); err != nil {
		t.Fatalf("Unmarshal() = %v", err)
	}

	tests := []struct {
		name  string
		want  interface{}
		blank interface{}
	}{{
		name:  "string list",
		want:  []string{"a", "b", "c"},
		blank: &[]string{},
	}, {
		name: "map string -> string",
		want: map[string]string{
			"a": "b",
			"c": "d",
		},
		blank: &map[string]string{},
	}, {
		name:  "nil pointer",
		want:  &Foo{},
		blank: &Foo{},
	}, {
		name:  "set pointer",
		want:  &Foo{Bar: &Bar{Field: "hello"}},
		blank: &Foo{},
	}, {
		name:  "image configuration",
		want:  want,
		blank: &types.ImageConfiguration{},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want, diags := generateValue(test.want)
			if diags.HasError() {
				t.Fatalf("generateValue() = %v", diags.Errors())
			}

			t.Logf("generateValue() = %#v", want)

			diags = assignValue(want, test.blank)
			if diags.HasError() {
				t.Fatalf("assignValue() = %v", diags.Errors())
			}

			got, diags := generateValue(test.blank)
			if diags.HasError() {
				t.Fatalf("generateValue() = %v", diags.Errors())
			}

			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
