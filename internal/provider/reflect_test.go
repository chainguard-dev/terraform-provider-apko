package provider

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
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
			got, err := generateType(reflect.TypeOf(test.obj))
			if err != nil {
				t.Fatalf("generateType() = %v", err)
			}
			if !test.want.Equal(got) {
				t.Errorf("got = %+v, wanted %v", got, test.want)
			}
		})
	}
}
