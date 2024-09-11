package provider

import (
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

func generateType(v any) (attr.Type, error) {
	return generateTypeReflect(reflect.TypeOf(v))
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
		ot := basetypes.ObjectType{
			AttrTypes: make(map[string]attr.Type, t.NumField()),
		}
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			tag := yamlName(sf)
			if tag == nil {
				continue
			}

			// HACK: Handle this field.
			if sf.Type.Kind() == reflect.Pointer {
				log.Println("skipping pointer field", sf.Name)
				continue
			}

			ft, err := generateTypeReflect(sf.Type)
			if err != nil {
				return nil, fmt.Errorf("struct %w", err)
			}
			ot.AttrTypes[*tag] = ft
		}
		return ot, nil

	default:
		return nil, fmt.Errorf("unknown type encountered: %v", t.Kind())
	}
}

func generateValue(v any) (attr.Value, diag.Diagnostics) {
	return generateValueReflect(reflect.ValueOf(v))
}

func generateValueReflect(v reflect.Value) (attr.Value, diag.Diagnostics) {
	t := v.Type()
	switch t.Kind() {
	case reflect.Pointer:
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

			// HACK: Handle this field.
			if sf.Type.Kind() == reflect.Pointer {
				log.Println("skipping pointer field", sf.Name)
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
			val, ok := fl[*tag]
			if !ok {
				continue
			}
			diags := assignValueReflect(val, out.Field(i))
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
