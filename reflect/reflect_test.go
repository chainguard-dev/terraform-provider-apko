package reflect

import (
	"testing"

	"chainguard.dev/apko/pkg/build/types"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"gopkg.in/yaml.v2"
)

type recursive struct {
	Foo   string
	Slice []recursive
	Map   map[string]recursive
	// Go doesn't support these recursive types.
	//Array [3]recursive
	//Struct recursive
}

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
			Pointer *string `yaml:"ptr"`
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
				"ptr": basetypes.StringType{},
			},
		},
	}, {
		name: "recursive object",
		obj: struct {
			Name   string
			Tagged int64 `yaml:"different"`
			Outer  []recursive
		}{},
		want: basetypes.ObjectType{
			AttrTypes: map[string]attr.Type{
				"name":      basetypes.StringType{},
				"different": basetypes.Int64Type{},
				"outer": basetypes.ListType{
					ElemType: basetypes.ObjectType{
						AttrTypes: map[string]attr.Type{
							"foo": basetypes.StringType{},
							"slice": basetypes.ListType{
								ElemType: basetypes.ObjectType{},
							},
							"map": basetypes.MapType{
								ElemType: basetypes.ObjectType{},
							},
						},
					},
				},
			},
		},
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := GenerateType(test.obj)
			if err != nil {
				t.Fatalf("GenerateType() = %v", err)
			}
			if !test.want.Equal(got) {
				t.Errorf("\n got = %+v\nwant = %v", got, test.want)
			}
		})
	}
}

func TestRoundtripValue(t *testing.T) {
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
		name:  "image configuration",
		want:  want,
		blank: &types.ImageConfiguration{},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want, diags := GenerateValue(test.want)
			if diags.HasError() {
				t.Fatalf("GenerateValue() = %v", diags.Errors())
			}

			t.Logf("GenerateValue() = %#v", want)

			diags = AssignValue(want, test.blank)
			if diags.HasError() {
				t.Fatalf("AssignValue() = %v", diags.Errors())
			}

			got, diags := GenerateValue(test.blank)
			if diags.HasError() {
				t.Fatalf("GenerateValue() = %v", diags.Errors())
			}

			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}
