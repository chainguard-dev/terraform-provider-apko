package provider

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// FieldMetadata contains Terraform schema properties extracted from struct tags
type FieldMetadata struct {
	Name        string    // Field name from yaml tag
	Type        attr.Type // Terraform attribute type
	Optional    bool      // Can be set in config (default: true)
	Required    bool      // Must be set in config
	Computed    bool      // Computed by provider
	Sensitive   bool      // Sensitive value (passwords, tokens)
	Description string    // Field description
}

// generateType converts a Go value to a Terraform attribute type using reflection.
//
// For struct types, fields are processed with tag-aware schema generation to ensure
// consistent handling of field properties. All structs (including nested structs) go
// through the same tag processing pipeline.
//
// Supported struct tags:
//
//	yaml:"field_name"      - Specifies the Terraform field name. Use yaml:"-" to skip a field.
//	tfgen:"required"       - Marks the field as required in Terraform config.
//	tfgen:"computed"       - Marks the field as computed by the provider.
//	tfgen:"optional"       - Explicitly marks the field as optional (default behavior).
//	tfgen:"sensitive"      - Marks the field as sensitive (e.g., passwords, tokens).
//	tfgen:"desc=..."       - Adds an inline description to the field.
//	apko:"experimental"    - Skips the field from schema generation entirely.
//
// Fields default to optional unless explicitly tagged with tfgen:"required" or tfgen:"computed".
// Multiple tfgen options can be combined with commas: tfgen:"optional,computed,sensitive"
//
// Examples:
//
//	type Config struct {
//	    Name     string `yaml:"name" tfgen:"required"`              // Required field
//	    Token    string `yaml:"token" tfgen:"optional,sensitive"`   // Optional + sensitive
//	    Status   string `yaml:"status" tfgen:"computed"`            // Computed field
//	    Optional string `yaml:"optional"`                           // Defaults to optional
//	    Internal string `yaml:"-"`                                  // Skipped
//	    Beta     string `yaml:"beta" apko:"experimental"`           // Skipped (experimental)
//	}
//
// The function recursively processes nested structs, slices, and maps to generate
// the complete Terraform schema type structure.
func generateType(v any) (attr.Type, error) {
	return generateTypeReflect(reflect.TypeOf(v))
}

// convertSchemaAttributeToType extracts attr.Type from a schema.Attribute
func convertSchemaAttributeToType(attr schema.Attribute) (attr.Type, error) {
	switch a := attr.(type) {
	case schema.StringAttribute:
		return basetypes.StringType{}, nil
	case schema.BoolAttribute:
		return basetypes.BoolType{}, nil
	case schema.Int64Attribute:
		return basetypes.Int64Type{}, nil
	case schema.Float64Attribute:
		return basetypes.Float64Type{}, nil
	case schema.ListAttribute:
		return basetypes.ListType{ElemType: a.ElementType}, nil
	case schema.MapAttribute:
		return basetypes.MapType{ElemType: a.ElementType}, nil
	case schema.ObjectAttribute:
		return basetypes.ObjectType{AttrTypes: a.AttributeTypes}, nil
	default:
		return nil, fmt.Errorf("unsupported schema attribute type: %T", attr)
	}
}

func generateTypeReflect(t reflect.Type) (attr.Type, error) {
	switch t.Kind() {
	case reflect.String:
		return basetypes.StringType{}, nil
	case reflect.Bool:
		return basetypes.BoolType{}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return basetypes.Int64Type{}, nil
	case reflect.Float32, reflect.Float64:
		return basetypes.Float64Type{}, nil

	case reflect.Array, reflect.Slice:
		st, err := generateTypeReflect(t.Elem())
		if err != nil {
			return nil, fmt.Errorf("[]%v: %w", t.Elem(), err)
		}
		return basetypes.ListType{
			ElemType: st,
		}, nil

	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("%v only string map keys are supported", t.Key())
		}
		et, err := generateTypeReflect(t.Elem())
		if err != nil {
			return nil, fmt.Errorf("map[string]%v: %w", t.Elem(), err)
		}
		return basetypes.MapType{
			ElemType: et,
		}, nil

	case reflect.Struct:
		// For structs, use generateSchemaAttributes to ensure consistent tag processing
		attrs, err := generateSchemaAttributes(t)
		if err != nil {
			return nil, err
		}

		// Convert schema.Attribute to attr.Type for each field
		attrTypes := make(map[string]attr.Type, len(attrs))
		for name, attr := range attrs {
			attrType, err := convertSchemaAttributeToType(attr)
			if err != nil {
				return nil, err
			}
			attrTypes[name] = attrType
		}

		return basetypes.ObjectType{AttrTypes: attrTypes}, nil
	case reflect.Pointer:
		return generateTypeReflect(maybeDeref(t))

	default:
		return nil, fmt.Errorf("unknown type encountered: %v", t.Kind())
	}
}

func maybeDeref(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Pointer {
		// For pointers we want the element's type, not Pointer.
		return t.Elem()
	}

	return t
}

func generateValue(v any) (attr.Value, diag.Diagnostics) {
	return generateValueReflect(reflect.ValueOf(v))
}

func generateNull(t reflect.Type, at attr.Type) (attr.Value, diag.Diagnostics) {
	switch t.Kind() {
	case reflect.String:
		return basetypes.NewStringNull(), nil
	case reflect.Bool:
		return basetypes.NewBoolNull(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return basetypes.NewInt64Null(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return basetypes.NewInt64Null(), nil
	case reflect.Float32, reflect.Float64:
		return basetypes.NewFloat64Null(), nil
	case reflect.Array, reflect.Slice:
		return basetypes.NewListNull(at), nil
	case reflect.Map:
		return basetypes.NewMapNull(at), nil
	case reflect.Struct:
		attrTyp, ok := at.(basetypes.ObjectType)
		if !ok {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("expected object type", "")}
		}
		return basetypes.NewObjectNull(attrTyp.AttrTypes), nil
	default:
		return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("unexpected null type", t.Kind().String())}
	}
}

func generateValueReflect(v reflect.Value) (attr.Value, diag.Diagnostics) {
	t := v.Type()
	switch t.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			st, err := generateTypeReflect(t.Elem())
			if err != nil {
				return nil, []diag.Diagnostic{diag.NewErrorDiagnostic(err.Error(), "")}
			}
			return generateNull(t, st)
		}
		return generateValueReflect(v.Elem())
	case reflect.String:
		return basetypes.NewStringValue(v.String()), nil
	case reflect.Bool:
		return basetypes.NewBoolValue(v.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return basetypes.NewInt64Value(v.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return basetypes.NewInt64Value(int64(v.Uint())), nil
	case reflect.Float32, reflect.Float64:
		return basetypes.NewFloat64Value(v.Float()), nil

	case reflect.Array, reflect.Slice:
		st, err := generateTypeReflect(t.Elem())
		if err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic(err.Error(), "")}
		}
		ets := make([]attr.Value, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			et, diags := generateValueReflect(v.Index(i))
			if diags.HasError() {
				return nil, diags
			}
			ets = append(ets, et)
		}
		return basetypes.NewListValue(st, ets)

	case reflect.Map:
		et, err := generateTypeReflect(t.Elem())
		if err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic(err.Error(), "")}
		}

		em := make(map[string]attr.Value, v.Len())
		for _, key := range v.MapKeys() {
			et, diags := generateValueReflect(v.MapIndex(key))
			if diags.HasError() {
				return nil, diags
			}
			em[key.String()] = et
		}
		return basetypes.NewMapValue(et, em)

	case reflect.Struct:
		ot, err := generateTypeReflect(t)
		if err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic(err.Error(), "")}
		}

		fv := make(map[string]attr.Value, t.NumField())
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			tag := yamlName(sf)
			if tag == nil {
				continue
			}
			if experimental(sf) {
				continue
			}

			if sf.Type.Kind() == reflect.Pointer && v.Field(i).IsNil() {
				at, err := generateTypeReflect(sf.Type)
				if err != nil {
					return nil, []diag.Diagnostic{diag.NewErrorDiagnostic(err.Error(), "")}
				}
				ft, diags := generateNull(sf.Type.Elem(), at)
				if diags.HasError() {
					return nil, diags
				}
				fv[*tag] = ft
				continue
			}

			ft, diags := generateValueReflect(v.Field(i))
			if diags.HasError() {
				return nil, diags
			}
			fv[*tag] = ft
		}

		attrTyp, ok := ot.(basetypes.ObjectType)
		if !ok {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("expected object type", "")}
		}

		return basetypes.NewObjectValue(attrTyp.AttrTypes, fv)

	default:
		return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("unknown type", t.Kind().String())}
	}
}

func assignValue(in attr.Value, out any) diag.Diagnostics {
	rv := reflect.ValueOf(out)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return []diag.Diagnostic{diag.NewErrorDiagnostic("not a pointer or nil", fmt.Sprintf("got: %T", out))}
	}

	// This is copied from encoding/json
	if rv.Kind() != reflect.Pointer && rv.Type().Name() != "" && rv.CanAddr() {
		rv = rv.Addr()
	}

	return assignValueReflect(in, rv)
}

func assignValueReflect(in attr.Value, out reflect.Value) diag.Diagnostics {
	t := out.Type()
	switch t.Kind() {
	case reflect.Pointer:
		return assignValueReflect(in, out.Elem())

	case reflect.String:
		sv, ok := in.(basetypes.StringValue)
		if !ok {
			return []diag.Diagnostic{diag.NewErrorDiagnostic("not a string", fmt.Sprintf("got: %T", in))}
		}
		out.SetString(sv.ValueString())
		return nil

	case reflect.Bool:
		bv, ok := in.(basetypes.BoolValue)
		if !ok {
			return []diag.Diagnostic{diag.NewErrorDiagnostic("not a bool", fmt.Sprintf("got: %T", in))}
		}
		out.SetBool(bv.ValueBool())
		return nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		iv, ok := in.(basetypes.Int64Value)
		if !ok {
			return []diag.Diagnostic{diag.NewErrorDiagnostic("not an int64", fmt.Sprintf("got: %T", in))}
		}
		out.SetInt(iv.ValueInt64())
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		iv, ok := in.(basetypes.Int64Value)
		if !ok {
			return []diag.Diagnostic{diag.NewErrorDiagnostic("not an int64", fmt.Sprintf("got: %T", in))}
		}
		out.SetUint(uint64(iv.ValueInt64()))
		return nil

	case reflect.Float32, reflect.Float64:
		fv, ok := in.(basetypes.Float64Value)
		if !ok {
			return []diag.Diagnostic{diag.NewErrorDiagnostic("not a float", fmt.Sprintf("got: %T", in))}
		}
		out.SetFloat(fv.ValueFloat64())
		return nil

	case reflect.Slice:
		lv, ok := in.(basetypes.ListValue)
		if !ok {
			return []diag.Diagnostic{diag.NewErrorDiagnostic("not a list", fmt.Sprintf("got: %T", in))}
		}
		elts := lv.Elements()

		out = indirect(out)
		t = out.Type()

		// Set the slice capacity appropriately (based on encoding/json)
		l := len(elts)
		if l > out.Cap() {
			newv := reflect.MakeSlice(t, out.Len(), l)
			reflect.Copy(newv, out)
			out.Set(newv)
		}
		if l >= out.Len() {
			out.SetLen(l)
		}

		// Copy into each of the elements.
		for i, elt := range elts {
			if diags := assignValueReflect(elt, out.Index(i)); diags.HasError() {
				return diags
			}
		}
		return nil

	case reflect.Map:
		mv, ok := in.(basetypes.MapValue)
		if !ok {
			return []diag.Diagnostic{diag.NewErrorDiagnostic("not a map", fmt.Sprintf("got: %T", in))}
		}
		elts := mv.Elements()

		out = indirect(out)
		t = out.Type()

		if out.IsNil() {
			out.Set(reflect.MakeMap(t))
		}

		for key, val := range elts {
			k := reflect.ValueOf(key)

			v := reflect.New(t.Elem()).Elem()
			if diags := assignValueReflect(val, v); diags.HasError() {
				return diags
			}
			out.SetMapIndex(k, v)
		}
		return nil

	case reflect.Struct:
		ov, ok := in.(basetypes.ObjectValue)
		if !ok {
			return []diag.Diagnostic{diag.NewErrorDiagnostic("not a object", fmt.Sprintf("got: %T", in))}
		}
		fl := ov.Attributes()

		out = indirect(out)
		t = out.Type()

		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			tag := yamlName(sf)
			if tag == nil {
				continue
			}
			if experimental(sf) {
				continue
			}
			val, ok := fl[*tag]
			if !ok {
				continue
			}

			if val.IsNull() {
				out.Field(i).Set(reflect.Zero(sf.Type))
				return nil
			}

			diags := assignValueReflect(val, indirect(out.Field(i)))
			if diags.HasError() {
				return diags
			}
			delete(fl, *tag)
		}

		diags := make([]diag.Diagnostic, 0, len(fl))
		for k := range fl {
			diags = append(diags, diag.NewErrorDiagnostic("unmatched field", fmt.Sprintf("%s was not found in struct", k)))
		}
		return diags

	default:
		return []diag.Diagnostic{diag.NewErrorDiagnostic("unknown type", t.Kind().String())}
	}
}

func yamlName(field reflect.StructField) *string {
	tag := field.Tag.Get("yaml")
	if tag == "" && !strings.Contains(string(field.Tag), ":") {
		tag = string(field.Tag)
	}
	if tag == "-" {
		return nil
	}
	fields := strings.Split(tag, ",")
	if fields[0] != "" {
		return &fields[0]
	}
	fn := strings.ToLower(field.Name)
	return &fn
}

func experimental(field reflect.StructField) bool {
	tag := field.Tag.Get("apko")
	if tag == "" {
		return false
	}
	for field := range strings.SplitSeq(tag, ",") {
		if field == "experimental" {
			return true
		}
	}
	return false
}

// extractFieldMetadata extracts Terraform schema metadata from a struct field's tags.
// Returns nil if the field should be skipped (experimental, unexported, etc.).
func extractFieldMetadata(field reflect.StructField) (*FieldMetadata, error) {
	// Parse yaml tag for field name
	name := yamlName(field)
	if name == nil {
		return nil, nil // Skip unexported fields or yaml:"-"
	}

	// Check for experimental flag (existing behavior)
	if experimental(field) {
		return nil, nil // Skip experimental fields
	}

	metadata := &FieldMetadata{
		Name:     *name,
		Optional: true, // Default to optional
	}

	// Parse tfgen tag for properties
	tfgenTag := field.Tag.Get("tfgen")
	if tfgenTag == "" {
		// No tfgen tag, use defaults (optional)
		return metadata, nil
	}

	// Parse comma-separated options
	hasExplicitOptional := false
	for _, opt := range strings.Split(tfgenTag, ",") {
		opt = strings.TrimSpace(opt)
		if opt == "" {
			continue
		}

		switch {
		case opt == "required":
			metadata.Required = true
			metadata.Optional = false
		case opt == "optional":
			hasExplicitOptional = true
			metadata.Optional = true
		case opt == "computed":
			metadata.Computed = true
		case opt == "sensitive":
			metadata.Sensitive = true
		case strings.HasPrefix(opt, "desc="):
			metadata.Description = strings.TrimPrefix(opt, "desc=")
		default:
			return nil, fmt.Errorf("unknown tfgen tag option: %s", opt)
		}
	}

	// Validation: required and optional are mutually exclusive
	if metadata.Required && hasExplicitOptional {
		return nil, fmt.Errorf("field %s: cannot be both required and optional", *name)
	}

	return metadata, nil
}

// metadataToSchemaAttribute converts FieldMetadata to a Terraform schema.Attribute.
func metadataToSchemaAttribute(meta *FieldMetadata) (schema.Attribute, error) {
	// Determine the attribute type and create appropriate schema
	switch t := meta.Type.(type) {
	case basetypes.StringType:
		return schema.StringAttribute{
			MarkdownDescription: meta.Description,
			Optional:            meta.Optional,
			Required:            meta.Required,
			Computed:            meta.Computed,
			Sensitive:           meta.Sensitive,
		}, nil

	case basetypes.BoolType:
		return schema.BoolAttribute{
			MarkdownDescription: meta.Description,
			Optional:            meta.Optional,
			Required:            meta.Required,
			Computed:            meta.Computed,
			Sensitive:           meta.Sensitive,
		}, nil

	case basetypes.Int64Type:
		return schema.Int64Attribute{
			MarkdownDescription: meta.Description,
			Optional:            meta.Optional,
			Required:            meta.Required,
			Computed:            meta.Computed,
			Sensitive:           meta.Sensitive,
		}, nil

	case basetypes.Float64Type:
		return schema.Float64Attribute{
			MarkdownDescription: meta.Description,
			Optional:            meta.Optional,
			Required:            meta.Required,
			Computed:            meta.Computed,
			Sensitive:           meta.Sensitive,
		}, nil

	case basetypes.ListType:
		return schema.ListAttribute{
			MarkdownDescription: meta.Description,
			Optional:            meta.Optional,
			Required:            meta.Required,
			Computed:            meta.Computed,
			Sensitive:           meta.Sensitive,
			ElementType:         t.ElemType,
		}, nil

	case basetypes.MapType:
		return schema.MapAttribute{
			MarkdownDescription: meta.Description,
			Optional:            meta.Optional,
			Required:            meta.Required,
			Computed:            meta.Computed,
			Sensitive:           meta.Sensitive,
			ElementType:         t.ElemType,
		}, nil

	case basetypes.ObjectType:
		return schema.ObjectAttribute{
			MarkdownDescription: meta.Description,
			Optional:            meta.Optional,
			Required:            meta.Required,
			Computed:            meta.Computed,
			Sensitive:           meta.Sensitive,
			AttributeTypes:      t.AttrTypes,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported attribute type: %T", meta.Type)
	}
}

// generateSchemaAttributes generates Terraform schema attributes from a struct type
// with full support for tfgen tags to control field properties.
//
// This function returns a map of Terraform schema.Attribute objects that can be used
// directly in Terraform provider schema definitions. It processes struct tags to
// automatically configure field properties such as Optional, Required, Computed, and Sensitive.
//
// Tag Processing:
//
//	yaml:"field_name"      - Determines the Terraform attribute name (required for all fields)
//	yaml:"-"               - Skips the field entirely
//	tfgen:"required"       - Makes the field required in Terraform configurations
//	tfgen:"computed"       - Marks the field as computed by the provider
//	tfgen:"optional"       - Explicitly marks as optional (default if no tfgen tag)
//	tfgen:"sensitive"      - Marks the field as sensitive (passwords, API keys, etc.)
//	tfgen:"desc=text"      - Sets the field's MarkdownDescription
//	apko:"experimental"    - Excludes the field from the schema
//
// Default Behavior:
//   - Fields without tfgen tags default to Optional: true
//   - Pointer types (*T) are treated the same as non-pointer types
//   - Nested structs are processed recursively with the same tag rules
//   - Experimental and unexported fields are automatically excluded
//
// Tag Combinations:
//   - Multiple properties can be comma-separated: tfgen:"optional,computed,sensitive"
//   - Cannot combine "required" with "optional" (validation error)
//   - Can combine "optional" with "computed" for fields that can be set or computed
//
// Example usage:
//
//	type MyResource struct {
//	    ID          string   `yaml:"id" tfgen:"computed"`
//	    Name        string   `yaml:"name" tfgen:"required,desc=The resource name"`
//	    Token       string   `yaml:"token" tfgen:"optional,sensitive"`
//	    Tags        []string `yaml:"tags"`                    // Defaults to optional
//	    Annotations map[string]string `yaml:"annotations"`   // Defaults to optional
//	    Internal    string   `yaml:"-"`                       // Skipped
//	}
//
//	attrs, err := generateSchemaAttributes(reflect.TypeOf(MyResource{}))
//	if err != nil {
//	    return err
//	}
//
//	resp.Schema = schema.Schema{
//	    Attributes: attrs,
//	}
//
// The function handles all Terraform-supported types including primitives (string, bool,
// int64, float64), collections (lists, maps), and nested objects.
func generateSchemaAttributes(t reflect.Type) (map[string]schema.Attribute, error) {
	// Dereference pointers
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	// Validate input is a struct
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got %v", t.Kind())
	}

	attrs := make(map[string]schema.Attribute, t.NumField())

	// Iterate through struct fields
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Extract metadata from tags
		meta, err := extractFieldMetadata(field)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Name, err)
		}
		if meta == nil {
			continue // Skip this field (experimental, unexported, etc.)
		}

		// Generate attribute type
		fieldType := maybeDeref(field.Type)
		attrType, err := generateTypeReflect(fieldType)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Name, err)
		}
		meta.Type = attrType

		// Convert to schema.Attribute
		attr, err := metadataToSchemaAttribute(meta)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Name, err)
		}

		attrs[meta.Name] = attr
	}

	return attrs, nil
}

// indirect walks down v allocating pointers as needed,
// until it gets to a non-pointer.
// If it encounters an Unmarshaler, indirect stops and returns that.
// If decodingNull is true, indirect stops at the first settable pointer so it
// can be set to nil.
// This is copied/modified from encoding/json.
func indirect(v reflect.Value) reflect.Value {
	// Issue #24153 indicates that it is generally not a guaranteed property
	// that you may round-trip a reflect.Value by calling Value.Addr().Elem()
	// and expect the value to still be settable for values derived from
	// unexported embedded struct fields.
	//
	// The logic below effectively does this when it first addresses the value
	// (to satisfy possible pointer methods) and continues to dereference
	// subsequent pointers as necessary.
	//
	// After the first round-trip, we set v back to the original value to
	// preserve the original RW flags contained in reflect.Value.
	v0 := v
	haveAddr := false

	// If v is a named type and is addressable,
	// start with its address, so that if the type has pointer methods,
	// we find them.
	if v.Kind() != reflect.Pointer && v.Type().Name() != "" && v.CanAddr() {
		haveAddr = true
		v = v.Addr()
	}
	for {
		// Load value from interface, but only if the result will be
		// usefully addressable.
		if v.Kind() == reflect.Interface && !v.IsNil() {
			e := v.Elem()
			if e.Kind() == reflect.Pointer && !e.IsNil() {
				haveAddr = false
				v = e
				continue
			}
		}

		if v.Kind() != reflect.Pointer {
			break
		}

		// Prevent infinite loop if v is an interface pointing to its own address:
		//     var v interface{}
		//     v = &v
		if v.Elem().Kind() == reflect.Interface && v.Elem().Elem() == v {
			v = v.Elem()
			break
		}
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}

		if haveAddr {
			v = v0 // restore original value after round-trip Value.Addr().Elem()
			haveAddr = false
		} else {
			v = v.Elem()
		}
	}
	return v
}
