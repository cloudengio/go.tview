// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package structmodel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
)

// ApplyOption configures how ApplyJSON and ApplyJSONFile apply JSON settings
// onto a target struct.
type ApplyOption func(*applyConfig)

type applyConfig struct {
	docOpts     []Option
	strictKeys  bool
	replaceMaps bool
}

// WithApplyOptions passes structmodel.Option values (such as WithCodec or
// WithEnumOptions) when building the schema for ApplyJSON.
func WithApplyOptions(opts ...Option) ApplyOption {
	return func(c *applyConfig) {
		c.docOpts = append(c.docOpts, opts...)
	}
}

// WithStrictJSON causes ApplyJSON to fail if the JSON contains keys not
// present in the target struct schema.
func WithStrictJSON(strict bool) ApplyOption {
	return func(c *applyConfig) {
		c.strictKeys = strict
	}
}

// WithReplaceMaps replaces map contents completely with the JSON object rather
// than merging keys (where null deletes a key).
func WithReplaceMaps(replace bool) ApplyOption {
	return func(c *applyConfig) {
		c.replaceMaps = replace
	}
}

// ApplyJSON applies JSON-encoded settings (either a full snapshot or a partial
// RFC 7396 merge patch) onto target, which must be a non-nil pointer to a
// struct. Fields omitted from JSON are preserved untouched on target.
func ApplyJSON(target any, data []byte, opts ...ApplyOption) error {
	if target == nil {
		return errors.New("target must not be nil")
	}
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("target must be a non-nil pointer to a struct, got %T", target)
	}

	var cfg applyConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	schema, _, err := prepareTarget(target, cfg.docOpts...)
	if err != nil {
		return err
	}
	raw, err := decodeRawJSON(data)
	if err != nil {
		return err
	}
	if raw == nil {
		return nil
	}
	return applyValue(schema.Root, newRootRef(rv), raw, schema.Codecs, &cfg)
}

// ApplyJSONFile reads a JSON settings file from path and applies it to target.
func ApplyJSONFile(target any, path string, opts ...ApplyOption) error {
	data, err := os.ReadFile(path) //nolint:gosec // caller-provided config path
	if err != nil {
		return err
	}
	return ApplyJSON(target, data, opts...)
}

func decodeRawJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func applyValue(desc *TypeDesc, ref ValueRef, raw any, codecs *CodecRegistry, cfg *applyConfig) error {
	switch desc.Kind {
	case KindScalar:
		return applyScalar(desc, ref, raw, codecs)
	case KindPtr:
		return applyPtr(desc, ref, raw, codecs, cfg)
	case KindStruct:
		return applyStruct(desc, ref, raw, codecs, cfg)
	case KindSlice:
		return applySlice(desc, ref, raw, codecs, cfg)
	case KindArray:
		return applyArray(desc, ref, raw, codecs, cfg)
	case KindMap:
		return applyMap(desc, ref, raw, codecs, cfg)
	default:
		return fmt.Errorf("unsupported schema kind %v", desc.Kind)
	}
}

func applyScalar(desc *TypeDesc, ref ValueRef, raw any, codecs *CodecRegistry) error {
	text, err := jsonScalarToText(raw)
	if err != nil {
		return fmt.Errorf("expected scalar for %s: %w", desc.Type, err)
	}
	parsed, err := codecs.Parse(desc.Type, text)
	if err != nil {
		return err
	}
	return ref.Set(parsed)
}

func jsonScalarToText(raw any) (string, error) {
	switch v := raw.(type) {
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	case bool:
		return strconv.FormatBool(v), nil
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	case nil:
		return "", errors.New("unexpected null for scalar")
	default:
		return "", fmt.Errorf("got %T", raw)
	}
}

func applyPtr(desc *TypeDesc, ref ValueRef, raw any, codecs *CodecRegistry, cfg *applyConfig) error {
	if raw == nil {
		return ref.Set(reflect.Zero(desc.Type))
	}
	cur := ref.Get()
	if !cur.IsValid() || cur.IsNil() {
		alloc := newDefaultValue(desc.Elem.Type, false)
		ptr := reflect.New(desc.Elem.Type)
		ptr.Elem().Set(alloc)
		if err := ref.Set(ptr); err != nil {
			return err
		}
	}
	return applyValue(desc.Elem, newPtrElemRef(ref, desc.Elem.Type), raw, codecs, cfg)
}

func applyStruct(desc *TypeDesc, ref ValueRef, raw any, codecs *CodecRegistry, cfg *applyConfig) error {
	obj, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("expected JSON object for %s, got %T", desc.Type, raw)
	}
	byName := make(map[string]*FieldDesc, len(desc.Fields))
	for _, f := range desc.Fields {
		byName[f.Tag.Name] = f
	}
	for k, v := range obj {
		f, ok := byName[k]
		if !ok {
			if cfg.strictKeys {
				return fmt.Errorf("unknown field %q on %s", k, desc.Type)
			}
			continue
		}
		if err := applyValue(f.Type, newFieldRef(ref, f.Path, f.Type.Type), v, codecs, cfg); err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
	}
	return nil
}

func applySlice(desc *TypeDesc, ref ValueRef, raw any, codecs *CodecRegistry, cfg *applyConfig) error {
	if raw == nil {
		return ref.Set(reflect.Zero(desc.Type))
	}
	arr, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("expected JSON array for %s, got %T", desc.Type, raw)
	}
	sl := reflect.MakeSlice(desc.Type, len(arr), len(arr))
	for i := range arr {
		sl.Index(i).Set(newDefaultValue(desc.Elem.Type, false))
	}
	if err := ref.Set(sl); err != nil {
		return err
	}
	for i, elem := range arr {
		if err := applyValue(desc.Elem, newSliceElemRef(ref, i, desc.Elem.Type), elem, codecs, cfg); err != nil {
			return fmt.Errorf("[%d]: %w", i, err)
		}
	}
	return nil
}

func applyArray(desc *TypeDesc, ref ValueRef, raw any, codecs *CodecRegistry, cfg *applyConfig) error {
	arr, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("expected JSON array for %s, got %T", desc.Type, raw)
	}
	n := desc.Type.Len()
	if len(arr) > n {
		return fmt.Errorf("JSON array length %d exceeds fixed array size %d", len(arr), n)
	}
	for i, elem := range arr {
		if err := applyValue(desc.Elem, newSliceElemRef(ref, i, desc.Elem.Type), elem, codecs, cfg); err != nil {
			return fmt.Errorf("[%d]: %w", i, err)
		}
	}
	return nil
}

func applyMap(desc *TypeDesc, ref ValueRef, raw any, codecs *CodecRegistry, cfg *applyConfig) error {
	if raw == nil {
		return ref.Set(reflect.Zero(desc.Type))
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("expected JSON object for %s, got %T", desc.Type, raw)
	}
	mp := ref.Get()
	if cfg.replaceMaps || !mp.IsValid() || mp.IsNil() {
		mp = reflect.MakeMap(desc.Type)
		if err := ref.Set(mp); err != nil {
			return err
		}
	}
	for k, v := range obj {
		rawKey, err := codecs.Parse(desc.Key.Type, k)
		if err != nil {
			return fmt.Errorf("map key %q: %w", k, err)
		}
		if v == nil {
			mp.SetMapIndex(rawKey, reflect.Value{})
			continue
		}
		if err := applyValue(desc.Elem, newMapEntryRef(ref, rawKey, desc.Elem.Type), v, codecs, cfg); err != nil {
			return fmt.Errorf("[%s]: %w", k, err)
		}
	}
	return nil
}
