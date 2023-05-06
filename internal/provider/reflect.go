package provider

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

func generateType(t reflect.Type) (attr.Type, error) {
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
		st, err := generateType(t.Elem())
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
		et, err := generateType(t.Elem())
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
			ft, err := generateType(sf.Type)
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

func generateValue(v reflect.Value) (attr.Value, diag.Diagnostics) {
	t := v.Type()
	switch t.Kind() {
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
		st, err := generateType(t.Elem())
		if err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic(err.Error(), "")}
		}
		ets := make([]attr.Value, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			et, diags := generateValue(v.Index(i))
			if diags.HasError() {
				return nil, diags
			}
			ets = append(ets, et)
		}
		return basetypes.NewListValue(st, ets)

	case reflect.Map:
		et, err := generateType(t.Elem())
		if err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic(err.Error(), "")}
		}
		em := make(map[string]attr.Value, v.Len())
		for _, key := range v.MapKeys() {
			et, diags := generateValue(v.MapIndex(key))
			if diags.HasError() {
				return nil, diags
			}
			em[key.String()] = et
		}
		return basetypes.NewMapValue(et, em)

	case reflect.Struct:
		ot, err := generateType(t)
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
			ft, diags := generateValue(v.Field(i))
			if diags.HasError() {
				return nil, diags
			}
			fv[*tag] = ft
		}
		return basetypes.NewObjectValue(ot.(basetypes.ObjectType).AttrTypes, fv)

	default:
		return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("unknown type", t.Kind().String())}
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
