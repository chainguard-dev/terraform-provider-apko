package provider

import (
	"reflect"
	"testing"

	"chainguard.dev/apko/pkg/build/types"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"gopkg.in/yaml.v2"
)

func TestGenerateType(t *testing.T) {
	tests := []struct {
		name string
		obj  any
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
		want  any
		blank any
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

func TestExtractFieldMetadata(t *testing.T) {
	tests := []struct {
		name    string
		field   reflect.StructField
		want    *FieldMetadata
		wantErr bool
	}{
		{
			name: "default optional",
			field: reflect.TypeOf(struct {
				Field string `yaml:"field"`
			}{}).Field(0),
			want: &FieldMetadata{
				Name:     "field",
				Optional: true,
			},
		},
		{
			name: "explicit required",
			field: reflect.TypeOf(struct {
				Field string `yaml:"field" tfgen:"required"`
			}{}).Field(0),
			want: &FieldMetadata{
				Name:     "field",
				Required: true,
				Optional: false,
			},
		},
		{
			name: "explicit computed",
			field: reflect.TypeOf(struct {
				Field string `yaml:"field" tfgen:"computed"`
			}{}).Field(0),
			want: &FieldMetadata{
				Name:     "field",
				Optional: true,
				Computed: true,
			},
		},
		{
			name: "optional and computed",
			field: reflect.TypeOf(struct {
				Field string `yaml:"field" tfgen:"optional,computed"`
			}{}).Field(0),
			want: &FieldMetadata{
				Name:     "field",
				Optional: true,
				Computed: true,
			},
		},
		{
			name: "required and sensitive",
			field: reflect.TypeOf(struct {
				Field string `yaml:"field" tfgen:"required,sensitive"`
			}{}).Field(0),
			want: &FieldMetadata{
				Name:      "field",
				Required:  true,
				Optional:  false,
				Sensitive: true,
			},
		},
		{
			name: "optional and sensitive",
			field: reflect.TypeOf(struct {
				Field *string `yaml:"field" tfgen:"optional,sensitive"`
			}{}).Field(0),
			want: &FieldMetadata{
				Name:      "field",
				Optional:  true,
				Sensitive: true,
			},
		},
		{
			name: "with description",
			field: reflect.TypeOf(struct {
				Field string `yaml:"field" tfgen:"required,desc=A required field"`
			}{}).Field(0),
			want: &FieldMetadata{
				Name:        "field",
				Required:    true,
				Optional:    false,
				Description: "A required field",
			},
		},
		{
			name: "experimental (skipped)",
			field: reflect.TypeOf(struct {
				Field string `yaml:"field" apko:"experimental"`
			}{}).Field(0),
			want: nil,
		},
		{
			name: "yaml skip (skipped)",
			field: reflect.TypeOf(struct {
				Field string `yaml:"-"`
			}{}).Field(0),
			want: nil,
		},
		{
			name: "pointer field defaults to optional",
			field: reflect.TypeOf(struct {
				Field *string `yaml:"field"`
			}{}).Field(0),
			want: &FieldMetadata{
				Name:     "field",
				Optional: true,
			},
		},
		{
			name: "required and optional conflict",
			field: reflect.TypeOf(struct {
				Field string `yaml:"field" tfgen:"required,optional"`
			}{}).Field(0),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractFieldMetadata(tt.field)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractFieldMetadata() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Compare without Type field (it's not set by extractFieldMetadata)
			// Clear Type field for comparison
			if got != nil {
				got.Type = nil
			}
			if tt.want != nil {
				tt.want.Type = nil
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("extractFieldMetadata() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGenerateSchemaAttributes(t *testing.T) {
	type TestStruct struct {
		DefaultField  string            `yaml:"default_field"`
		RequiredField string            `yaml:"required_field" tfgen:"required"`
		ComputedField string            `yaml:"computed_field" tfgen:"computed"`
		OptComputed   string            `yaml:"opt_computed" tfgen:"optional,computed"`
		Password      string            `yaml:"password" tfgen:"required,sensitive"`
		OptionalToken *string           `yaml:"token" tfgen:"optional,sensitive"`
		WithDesc      string            `yaml:"with_desc" tfgen:"required,desc=A required field"`
		Experimental  string            `yaml:"experimental" apko:"experimental"`
		Skipped       string            `yaml:"-"`
		IntField      int64             `yaml:"int_field"`
		BoolField     bool              `yaml:"bool_field"`
		ListField     []string          `yaml:"list_field" tfgen:"required"`
		MapField      map[string]string `yaml:"map_field"`
	}

	got, err := generateSchemaAttributes(reflect.TypeOf(TestStruct{}))
	if err != nil {
		t.Fatalf("generateSchemaAttributes() error = %v", err)
	}
	want := map[string]schema.Attribute{
		"default_field": schema.StringAttribute{
			Optional: true,
		},
		"required_field": schema.StringAttribute{
			Required: true,
		},
		"computed_field": schema.StringAttribute{
			Optional: true,
			Computed: true,
		},
		"opt_computed": schema.StringAttribute{
			Optional: true,
			Computed: true,
		},
		"password": schema.StringAttribute{
			Required:  true,
			Sensitive: true,
		},
		"token": schema.StringAttribute{
			Optional:  true,
			Sensitive: true,
		},
		"with_desc": schema.StringAttribute{
			MarkdownDescription: "A required field",
			Required:            true,
		},
		"int_field": schema.Int64Attribute{
			Optional: true,
		},
		"bool_field": schema.BoolAttribute{
			Optional: true,
		},
		"list_field": schema.ListAttribute{
			Required:    true,
			ElementType: basetypes.StringType{},
		},
		"map_field": schema.MapAttribute{
			Optional:    true,
			ElementType: basetypes.StringType{},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("generateSchemaAttributes() mismatch (-want +got):\n%s", diff)
	}
}

func TestGenerateSchemaAttributesWithNested(t *testing.T) {
	type Inner struct {
		InnerField string `yaml:"inner_field" tfgen:"required"`
	}

	type Outer struct {
		OuterField string `yaml:"outer_field"`
		Nested     Inner  `yaml:"nested"`
	}

	got, err := generateSchemaAttributes(reflect.TypeOf(Outer{}))
	if err != nil {
		t.Fatalf("generateSchemaAttributes() error = %v", err)
	}

	want := map[string]schema.Attribute{
		"outer_field": schema.StringAttribute{
			Optional: true,
		},
		"nested": schema.ObjectAttribute{
			Optional: true,
			AttributeTypes: map[string]attr.Type{
				"inner_field": basetypes.StringType{},
			},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("generateSchemaAttributes() with nested mismatch (-want +got):\n%s", diff)
	}
}

func TestGenerateSchemaAttributesErrors(t *testing.T) {
	tests := []struct {
		name    string
		typ     reflect.Type
		wantErr bool
	}{
		{
			name:    "non-struct",
			typ:     reflect.TypeOf("string"),
			wantErr: true,
		},
		{
			name: "invalid tag combination",
			typ: reflect.TypeOf(struct {
				Field string `yaml:"field" tfgen:"required,optional"`
			}{}),
			wantErr: true,
		},
		{
			name: "unknown tag option",
			typ: reflect.TypeOf(struct {
				Field string `yaml:"field" tfgen:"invalid_option"`
			}{}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := generateSchemaAttributes(tt.typ)
			if (err != nil) != tt.wantErr {
				t.Errorf("generateSchemaAttributes() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultOptionalBehavior(t *testing.T) {
	// Test that fields without tfgen tags default to optional
	type SimpleStruct struct {
		Field1 string  `yaml:"field1"`
		Field2 int64   `yaml:"field2"`
		Field3 *string `yaml:"field3"`
	}

	got, err := generateSchemaAttributes(reflect.TypeOf(SimpleStruct{}))
	if err != nil {
		t.Fatalf("generateSchemaAttributes() error = %v", err)
	}

	want := map[string]schema.Attribute{
		"field1": schema.StringAttribute{
			Optional: true,
		},
		"field2": schema.Int64Attribute{
			Optional: true,
		},
		"field3": schema.StringAttribute{
			Optional: true,
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("default optional behavior mismatch (-want +got):\n%s", diff)
	}
}

func TestNestedStructTagProcessing(t *testing.T) {
	// Test that nested structs also get tag processing (optional-by-default behavior)
	type Inner struct {
		InnerField1 string `yaml:"inner_field1"`
		InnerField2 string `yaml:"inner_field2" tfgen:"required"`
	}

	type Outer struct {
		OuterField string `yaml:"outer_field"`
		Nested     Inner  `yaml:"nested"`
	}

	// Test via generateType to ensure nested structs go through tag processing
	got, err := generateType(Outer{})
	if err != nil {
		t.Fatalf("generateType() error = %v", err)
	}

	outerType, ok := got.(basetypes.ObjectType)
	if !ok {
		t.Fatalf("expected ObjectType, got %T", got)
	}

	// Verify the nested struct was processed
	nestedType, ok := outerType.AttrTypes["nested"].(basetypes.ObjectType)
	if !ok {
		t.Fatalf("expected nested to be ObjectType, got %T", outerType.AttrTypes["nested"])
	}

	// Both fields should be present (tag processing worked)
	if _, ok := nestedType.AttrTypes["inner_field1"]; !ok {
		t.Error("inner_field1 should be present (optional-by-default)")
	}
	if _, ok := nestedType.AttrTypes["inner_field2"]; !ok {
		t.Error("inner_field2 should be present (marked required)")
	}

	// Verify experimental fields are still skipped in nested structs
	type InnerWithExperimental struct {
		NormalField       string `yaml:"normal_field"`
		ExperimentalField string `yaml:"experimental_field" apko:"experimental"`
	}

	type OuterWithExperimental struct {
		Nested InnerWithExperimental `yaml:"nested"`
	}

	got2, err := generateType(OuterWithExperimental{})
	if err != nil {
		t.Fatalf("generateType() error = %v", err)
	}

	outerType2, ok := got2.(basetypes.ObjectType)
	if !ok {
		t.Fatalf("expected ObjectType, got %T", got2)
	}

	nestedType2, ok := outerType2.AttrTypes["nested"].(basetypes.ObjectType)
	if !ok {
		t.Fatalf("expected nested to be ObjectType, got %T", outerType2.AttrTypes["nested"])
	}

	// Normal field should be present
	if _, ok := nestedType2.AttrTypes["normal_field"]; !ok {
		t.Error("normal_field should be present")
	}

	// Experimental field should be skipped
	if _, ok := nestedType2.AttrTypes["experimental_field"]; ok {
		t.Error("experimental_field should be skipped in nested struct")
	}
}
