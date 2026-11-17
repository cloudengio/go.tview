// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package structmodel

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// EnumOptionProvider may be implemented by enum types to supply their allowed
// string representations directly.
type EnumOptionProvider interface {
	EnumOptions() []string
}

var enumOptionProviderType = reflect.TypeFor[EnumOptionProvider]()

// ExtractEnumOptions inspects t for enum value metadata and returns the sorted
// list of allowed option strings if t represents an enum.
//
// It recognizes:
//   - types implementing EnumOptionProvider (EnumOptions() []string)
//   - types implementing EnumValues() map[string]T (matching cmdutil/flags.EnumType[T])
//   - wrapper structs with a Value field whose type implements EnumValues() map[string]T
//     or with an AllowedValues() string method (matching cmdutil/flags.Enum[T])
func ExtractEnumOptions(t reflect.Type) ([]string, bool) {
	if t == nil {
		return nil, false
	}
	if opts, ok := extractViaProvider(t); ok {
		return opts, true
	}
	if keys, _, ok := readEnumValuesMap(reflect.Zero(t)); ok {
		return keys, true
	}
	if t.Kind() == reflect.Struct {
		if vf, ok := t.FieldByName("Value"); ok {
			if keys, _, ok := readEnumValuesMap(reflect.Zero(vf.Type)); ok {
				return keys, true
			}
		}
	}
	if opts, ok := extractViaAllowedValues(t); ok {
		return opts, true
	}
	return nil, false
}

func extractViaProvider(t reflect.Type) ([]string, bool) {
	if t.Implements(enumOptionProviderType) {
		p, ok := reflect.Zero(t).Interface().(EnumOptionProvider)
		if ok {
			return copyAndSort(p.EnumOptions()), true
		}
	}
	if reflect.PointerTo(t).Implements(enumOptionProviderType) {
		p, ok := reflect.New(t).Interface().(EnumOptionProvider)
		if ok {
			return copyAndSort(p.EnumOptions()), true
		}
	}
	return nil, false
}

func extractViaAllowedValues(t reflect.Type) ([]string, bool) {
	m, ok := t.MethodByName("AllowedValues")
	if !ok || m.Type.NumIn() != 1 || m.Type.NumOut() != 1 || m.Type.Out(0).Kind() != reflect.String {
		return nil, false
	}
	res := reflect.Zero(t).MethodByName("AllowedValues").Call(nil)
	raw := res[0].String()
	if strings.TrimSpace(raw) == "" {
		return nil, false
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	sort.Strings(out)
	return out, len(out) > 0
}

func hasEnumValuesMethod(t reflect.Type) bool {
	m, ok := t.MethodByName("EnumValues")
	if !ok || m.Type.NumIn() != 1 || m.Type.NumOut() != 1 {
		return false
	}
	out := m.Type.Out(0)
	return out.Kind() == reflect.Map && out.Key().Kind() == reflect.String
}

func readEnumValuesMap(target reflect.Value) ([]string, reflect.Value, bool) {
	if !target.IsValid() || !hasEnumValuesMethod(target.Type()) {
		return nil, reflect.Value{}, false
	}
	res := target.MethodByName("EnumValues").Call(nil)
	mp := res[0]
	if !mp.IsValid() || mp.IsNil() || mp.Len() == 0 {
		return nil, mp, false
	}
	keys := make([]string, 0, mp.Len())
	iter := mp.MapRange()
	for iter.Next() {
		keys = append(keys, iter.Key().String())
	}
	sort.Strings(keys)
	return keys, mp, true
}

func formatEnumValue(v reflect.Value) (string, bool, error) {
	keys, mp, ok := readEnumValuesMap(v)
	if !ok {
		return "", false, nil
	}
	for _, k := range keys {
		mv := mp.MapIndex(reflect.ValueOf(k).Convert(mp.Type().Key()))
		if mv.IsValid() && mv.Type().Comparable() && v.Type().Comparable() && mv.Interface() == v.Interface() {
			return k, true, nil
		}
	}
	if s, ok := v.Interface().(fmt.Stringer); ok {
		return s.String(), true, nil
	}
	return fmt.Sprintf("%v", v.Interface()), true, nil
}

func parseEnumValue(t reflect.Type, text string) (reflect.Value, bool, error) {
	keys, mp, ok := readEnumValuesMap(reflect.Zero(t))
	if !ok {
		return reflect.Value{}, false, nil
	}
	mv := mp.MapIndex(reflect.ValueOf(text).Convert(mp.Type().Key()))
	if !mv.IsValid() {
		return reflect.Value{}, true, fmt.Errorf("invalid value %q (must be one of: %s)", text, strings.Join(keys, ", "))
	}
	if mv.Type().AssignableTo(t) {
		return mv, true, nil
	}
	if mv.Type().ConvertibleTo(t) {
		return mv.Convert(t), true, nil
	}
	return reflect.Value{}, true, fmt.Errorf("enum value type %s not assignable to %s", mv.Type(), t)
}

func copyAndSort(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
