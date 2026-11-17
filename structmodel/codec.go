// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

// Package structmodel provides reflection-based schema construction, value
// bindings, scalar codecs, and lazy tree navigation for arbitrary Go structs
// annotated with yaml and tview struct tags.
package structmodel

import (
	"encoding"
	"flag"
	"fmt"
	"reflect"
	"strconv"
	"time"
)

// ScalarKind identifies the broad scalar category of a type for widget
// selection and formatting.
type ScalarKind int

const (
	ScalarUnknown ScalarKind = iota
	ScalarString
	ScalarBool
	ScalarInt
	ScalarUint
	ScalarFloat
	ScalarDuration
	ScalarText
	ScalarFlag
	ScalarEnum
	ScalarCustom
)

// Codec formats and parses values of a scalar Go type.
type Codec interface {
	Format(v reflect.Value) (string, error)
	Parse(t reflect.Type, text string) (reflect.Value, error)
}

// CodecRegistry maps specific Go reflect.Types to custom Codecs and handles
// built-in primitive and encoding.TextMarshaler/TextUnmarshaler types.
type CodecRegistry struct {
	custom map[reflect.Type]Codec
}

// NewCodecRegistry returns an empty CodecRegistry.
func NewCodecRegistry() *CodecRegistry {
	return &CodecRegistry{
		custom: make(map[reflect.Type]Codec),
	}
}

// Register associates a custom Codec with t.
func (r *CodecRegistry) Register(t reflect.Type, c Codec) {
	if r.custom == nil {
		r.custom = make(map[reflect.Type]Codec)
	}
	r.custom[t] = c
}

// Classify returns whether t can be edited as a scalar and its ScalarKind.
func (r *CodecRegistry) Classify(t reflect.Type) (ScalarKind, bool) {
	if r != nil && r.custom[t] != nil {
		return ScalarCustom, true
	}
	if t == reflect.TypeFor[time.Duration]() {
		return ScalarDuration, true
	}
	if isTextCodecType(t) {
		return ScalarText, true
	}
	if isFlagValueCodecType(t) {
		return ScalarFlag, true
	}
	if hasEnumValuesMethod(t) {
		return ScalarEnum, true
	}
	return classifyPrimitive(t)
}

// Format converts v into its string representation.
func (r *CodecRegistry) Format(v reflect.Value) (string, error) {
	if !v.IsValid() {
		return "", nil
	}
	t := v.Type()
	if r != nil {
		if c, ok := r.custom[t]; ok {
			return c.Format(v)
		}
	}
	if t == reflect.TypeFor[time.Duration]() {
		return time.Duration(v.Int()).String(), nil
	}
	if s, ok, err := formatTextMarshaler(v); ok {
		return s, err
	}
	if s, ok := formatFlagValue(v); ok {
		return s, nil
	}
	if s, ok, err := formatEnumValue(v); ok {
		return s, err
	}
	return formatPrimitive(v)
}

// Parse converts text into a reflect.Value assignable to t.
func (r *CodecRegistry) Parse(t reflect.Type, text string) (reflect.Value, error) {
	if r != nil {
		if c, ok := r.custom[t]; ok {
			return c.Parse(t, text)
		}
	}
	if t == reflect.TypeFor[time.Duration]() {
		d, err := time.ParseDuration(text)
		if err != nil {
			return reflect.Value{}, err
		}
		out := reflect.New(t).Elem()
		out.SetInt(int64(d))
		return out, nil
	}
	if v, ok, err := parseTextUnmarshaler(t, text); ok {
		return v, err
	}
	if v, ok, err := parseFlagValue(t, text); ok {
		return v, err
	}
	if v, ok, err := parseEnumValue(t, text); ok {
		return v, err
	}
	return parsePrimitive(t, text)
}

func classifyPrimitive(t reflect.Type) (ScalarKind, bool) {
	switch t.Kind() {
	case reflect.String:
		return ScalarString, true
	case reflect.Bool:
		return ScalarBool, true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return ScalarInt, true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return ScalarUint, true
	case reflect.Float32, reflect.Float64:
		return ScalarFloat, true
	default:
		return ScalarUnknown, false
	}
}

var (
	textMarshalerType   = reflect.TypeFor[encoding.TextMarshaler]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
	flagValueType       = reflect.TypeFor[flag.Value]()
)

func isFlagValueCodecType(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).IsExported() {
			return false
		}
	}
	return reflect.PointerTo(t).Implements(flagValueType)
}

func formatFlagValue(v reflect.Value) (string, bool) {
	if !isFlagValueCodecType(v.Type()) {
		return "", false
	}
	ptr := reflect.New(v.Type())
	ptr.Elem().Set(v)
	fv, ok := ptr.Interface().(flag.Value)
	if !ok {
		return "", false
	}
	return fv.String(), true
}

func parseFlagValue(t reflect.Type, text string) (reflect.Value, bool, error) {
	if !isFlagValueCodecType(t) {
		return reflect.Value{}, false, nil
	}
	ptr := reflect.New(t)
	fv, ok := ptr.Interface().(flag.Value)
	if !ok {
		return reflect.Value{}, false, nil
	}
	if err := fv.Set(text); err != nil {
		return reflect.Value{}, true, err
	}
	return ptr.Elem(), true, nil
}

func isTextCodecType(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		return false
	}
	hasMarshal := t.Implements(textMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType)
	hasUnmarshal := reflect.PointerTo(t).Implements(textUnmarshalerType)
	return hasMarshal && hasUnmarshal
}

func formatTextMarshaler(v reflect.Value) (string, bool, error) {
	if !isTextCodecType(v.Type()) {
		return "", false, nil
	}
	if v.Type().Implements(textMarshalerType) {
		m, ok := v.Interface().(encoding.TextMarshaler)
		if !ok {
			return "", false, nil
		}
		b, err := m.MarshalText()
		return string(b), true, err
	}
	ptr := reflect.New(v.Type())
	ptr.Elem().Set(v)
	m, ok := ptr.Interface().(encoding.TextMarshaler)
	if !ok {
		return "", false, nil
	}
	b, err := m.MarshalText()
	return string(b), true, err
}

func parseTextUnmarshaler(t reflect.Type, text string) (reflect.Value, bool, error) {
	if !isTextCodecType(t) {
		return reflect.Value{}, false, nil
	}
	ptr := reflect.New(t)
	u, ok := ptr.Interface().(encoding.TextUnmarshaler)
	if !ok {
		return reflect.Value{}, false, nil
	}
	if err := u.UnmarshalText([]byte(text)); err != nil {
		return reflect.Value{}, true, err
	}
	return ptr.Elem(), true, nil
}

func formatPrimitive(v reflect.Value) (string, error) {
	switch v.Kind() {
	case reflect.String:
		return v.String(), nil
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(v.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, v.Type().Bits()), nil
	default:
		return "", fmt.Errorf("unsupported scalar type %s", v.Type())
	}
}

func parsePrimitive(t reflect.Type, text string) (reflect.Value, error) {
	out := reflect.New(t).Elem()
	switch t.Kind() {
	case reflect.String:
		out.SetString(text)
		return out, nil
	case reflect.Bool:
		b, err := strconv.ParseBool(text)
		if err != nil {
			return reflect.Value{}, err
		}
		out.SetBool(b)
		return out, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(text, 10, t.Bits())
		if err != nil {
			return reflect.Value{}, err
		}
		out.SetInt(n)
		return out, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n, err := strconv.ParseUint(text, 10, t.Bits())
		if err != nil {
			return reflect.Value{}, err
		}
		out.SetUint(n)
		return out, nil
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(text, t.Bits())
		if err != nil {
			return reflect.Value{}, err
		}
		out.SetFloat(f)
		return out, nil
	default:
		return reflect.Value{}, fmt.Errorf("unsupported scalar type %s", t)
	}
}
