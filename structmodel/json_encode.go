// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package structmodel

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

// MarshalJSON encodes target (a struct or pointer to struct) into indented JSON
// using the same yaml/tview field names, inline flattening, visibility rules,
// and scalar codecs used by the editor.
func MarshalJSON(target any, opts ...Option) ([]byte, error) {
	schema, rv, err := prepareTarget(target, opts...)
	if err != nil {
		return nil, err
	}
	seen := make(map[copyVisitKey]bool)
	encoded, err := encodeValue(schema.Root, rv, schema.Codecs, seen)
	if err != nil {
		return nil, err
	}
	return marshalIndented(encoded)
}

// SaveJSONFile encodes target with MarshalJSON and writes it atomically to path.
func SaveJSONFile(path string, target any, opts ...Option) error {
	data, err := MarshalJSON(target, opts...)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

// MarshalOverridesJSON encodes only the fields and entries in updated that
// differ from base as an RFC 7396 JSON merge patch. If nothing differs, it
// returns "{}\n".
func MarshalOverridesJSON(base, updated any, opts ...Option) ([]byte, error) {
	schema, baseVal, err := prepareTarget(base, opts...)
	if err != nil {
		return nil, fmt.Errorf("base: %w", err)
	}
	_, updVal, err := prepareTarget(updated, opts...)
	if err != nil {
		return nil, fmt.Errorf("updated: %w", err)
	}
	if baseVal.Type() != updVal.Type() {
		return nil, fmt.Errorf("base type %s does not match updated type %s", baseVal.Type(), updVal.Type())
	}

	diff, changed, err := diffValues(schema.Root, baseVal, updVal, schema.Codecs, make(map[copyVisitKey]bool))
	if err != nil {
		return nil, err
	}
	if !changed {
		return []byte("{}\n"), nil
	}
	return marshalIndented(diff)
}

// SaveOverridesJSONFile computes MarshalOverridesJSON(base, updated), merges
// it onto any existing JSON file at path (if present), and writes the result.
func SaveOverridesJSONFile(path string, base, updated any, opts ...Option) error {
	patch, err := MarshalOverridesJSON(base, updated, opts...)
	if err != nil {
		return err
	}
	existing, err := os.ReadFile(path) //nolint:gosec // caller-provided config path
	if err == nil && len(existing) > 0 {
		patch, err = MergeJSON(existing, patch)
		if err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeFileAtomic(path, patch)
}

// OverridesJSON encodes only the changes made in this Document relative to
// the original target struct.
func (d *Document) OverridesJSON() ([]byte, error) {
	diff, changed, err := diffValues(
		d.schema.Root,
		d.targetPtr.Elem(),
		d.workingPtr.Elem(),
		d.schema.Codecs,
		make(map[copyVisitKey]bool),
	)
	if err != nil {
		return nil, err
	}
	if !changed {
		return []byte("{}\n"), nil
	}
	return marshalIndented(diff)
}

// MergeJSON applies an RFC 7396 JSON merge patch onto baseJSON.
func MergeJSON(baseJSON, patchJSON []byte) ([]byte, error) {
	baseVal, err := decodeRawJSON(baseJSON)
	if err != nil {
		return nil, fmt.Errorf("base json: %w", err)
	}
	patchVal, err := decodeRawJSON(patchJSON)
	if err != nil {
		return nil, fmt.Errorf("patch json: %w", err)
	}
	merged := mergePatchValues(baseVal, patchVal)
	return marshalIndented(merged)
}

func mergePatchValues(target, patch any) any {
	pm, ok := patch.(map[string]any)
	if !ok {
		return patch
	}
	tm, ok := target.(map[string]any)
	if !ok {
		tm = make(map[string]any)
	}
	out := make(map[string]any, len(tm))
	for k, v := range tm {
		out[k] = v
	}
	for k, v := range pm {
		if v == nil {
			delete(out, k)
			continue
		}
		out[k] = mergePatchValues(out[k], v)
	}
	return out
}

func prepareTarget(target any, opts ...Option) (*Schema, reflect.Value, error) {
	if target == nil {
		return nil, reflect.Value{}, errors.New("target must not be nil")
	}
	rv := reflect.ValueOf(target)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, reflect.Value{}, errors.New("target pointer must not be nil")
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, reflect.Value{}, fmt.Errorf("expected struct or pointer to struct, got %T", target)
	}

	cfg := &docConfig{codecs: NewCodecRegistry()}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	schema, err := BuildSchemaWithEnums(rv.Type(), cfg.codecs, cfg.enumOverrides)
	if err != nil {
		return nil, reflect.Value{}, err
	}
	return schema, rv, nil
}

func encodeValue(desc *TypeDesc, v reflect.Value, codecs *CodecRegistry, seen map[copyVisitKey]bool) (any, error) {
	switch desc.Kind {
	case KindScalar:
		return encodeScalar(v, desc, codecs)
	case KindPtr:
		return encodePtr(desc, v, codecs, seen)
	case KindStruct:
		return encodeStruct(desc, v, codecs, seen)
	case KindSlice:
		if v.IsNil() {
			return nil, nil
		}
		return encodeSequence(desc.Elem, v, codecs, seen)
	case KindArray:
		return encodeSequence(desc.Elem, v, codecs, seen)
	case KindMap:
		return encodeMap(desc, v, codecs, seen)
	default:
		return nil, fmt.Errorf("unsupported schema kind %v", desc.Kind)
	}
}

func encodePtr(desc *TypeDesc, v reflect.Value, codecs *CodecRegistry, seen map[copyVisitKey]bool) (any, error) {
	if !v.IsValid() || v.IsNil() {
		return nil, nil
	}
	key := copyVisitKey{ptr: v.Pointer(), typ: v.Type()}
	if seen[key] {
		return nil, nil
	}
	seen[key] = true
	defer delete(seen, key)
	return encodeValue(desc.Elem, v.Elem(), codecs, seen)
}

func encodeStruct(desc *TypeDesc, v reflect.Value, codecs *CodecRegistry, seen map[copyVisitKey]bool) (map[string]any, error) {
	out := make(map[string]any, len(desc.Fields))
	for _, f := range desc.Fields {
		fv, ok := readFieldPath(v, f.Path)
		if !ok {
			out[f.Tag.Name] = nil
			continue
		}
		val, err := encodeValue(f.Type, fv, codecs, seen)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Tag.Name, err)
		}
		out[f.Tag.Name] = val
	}
	return out, nil
}

func encodeSequence(elemDesc *TypeDesc, v reflect.Value, codecs *CodecRegistry, seen map[copyVisitKey]bool) ([]any, error) {
	out := make([]any, v.Len())
	for i := 0; i < v.Len(); i++ {
		val, err := encodeValue(elemDesc, v.Index(i), codecs, seen)
		if err != nil {
			return nil, fmt.Errorf("[%d]: %w", i, err)
		}
		out[i] = val
	}
	return out, nil
}

func encodeMap(desc *TypeDesc, v reflect.Value, codecs *CodecRegistry, seen map[copyVisitKey]bool) (any, error) {
	if v.IsNil() {
		return nil, nil
	}
	out := make(map[string]any, v.Len())
	iter := v.MapRange()
	for iter.Next() {
		ks, err := codecs.Format(iter.Key())
		if err != nil {
			return nil, err
		}
		val, err := encodeValue(desc.Elem, iter.Value(), codecs, seen)
		if err != nil {
			return nil, fmt.Errorf("[%s]: %w", ks, err)
		}
		out[ks] = val
	}
	return out, nil
}

func encodeScalar(v reflect.Value, desc *TypeDesc, codecs *CodecRegistry) (any, error) {
	switch desc.ScalarKind {
	case ScalarBool:
		return v.Bool(), nil
	case ScalarInt:
		return v.Int(), nil
	case ScalarUint:
		return v.Uint(), nil
	case ScalarFloat:
		return v.Float(), nil
	default:
		return codecs.Format(v)
	}
}

func diffValues(desc *TypeDesc, base, upd reflect.Value, codecs *CodecRegistry, seen map[copyVisitKey]bool) (any, bool, error) {
	switch desc.Kind {
	case KindScalar:
		return diffScalar(desc, base, upd, codecs)
	case KindPtr:
		return diffPtr(desc, base, upd, codecs, seen)
	case KindStruct:
		return diffStruct(desc, base, upd, codecs, seen)
	case KindSlice, KindArray:
		return diffSequence(desc, base, upd, codecs, seen)
	case KindMap:
		return diffMap(desc, base, upd, codecs, seen)
	default:
		return nil, false, nil
	}
}

func diffScalar(desc *TypeDesc, base, upd reflect.Value, codecs *CodecRegistry) (any, bool, error) {
	bv, err := encodeScalar(base, desc, codecs)
	if err != nil {
		return nil, false, err
	}
	uv, err := encodeScalar(upd, desc, codecs)
	if err != nil {
		return nil, false, err
	}
	if reflect.DeepEqual(bv, uv) {
		return nil, false, nil
	}
	return uv, true, nil
}

func diffPtr(desc *TypeDesc, base, upd reflect.Value, codecs *CodecRegistry, seen map[copyVisitKey]bool) (any, bool, error) {
	baseNil := !base.IsValid() || base.IsNil()
	updNil := !upd.IsValid() || upd.IsNil()
	switch {
	case baseNil && updNil:
		return nil, false, nil
	case !baseNil && updNil:
		return nil, true, nil
	case baseNil && !updNil:
		val, err := encodeValue(desc.Elem, upd.Elem(), codecs, seen)
		return val, true, err
	default:
		key := copyVisitKey{ptr: upd.Pointer(), typ: upd.Type()}
		if seen[key] {
			return nil, false, nil
		}
		seen[key] = true
		defer delete(seen, key)
		return diffValues(desc.Elem, base.Elem(), upd.Elem(), codecs, seen)
	}
}

func diffStruct(desc *TypeDesc, base, upd reflect.Value, codecs *CodecRegistry, seen map[copyVisitKey]bool) (any, bool, error) {
	out := make(map[string]any)
	for _, f := range desc.Fields {
		bv, bok := readFieldPath(base, f.Path)
		uv, uok := readFieldPath(upd, f.Path)
		if !bok && !uok {
			continue
		}
		if !bok {
			bv = reflect.Zero(f.Type.Type)
		}
		if !uok {
			uv = reflect.Zero(f.Type.Type)
		}
		val, changed, err := diffValues(f.Type, bv, uv, codecs, seen)
		if err != nil {
			return nil, false, fmt.Errorf("%s: %w", f.Tag.Name, err)
		}
		if changed {
			out[f.Tag.Name] = val
		}
	}
	if len(out) == 0 {
		return nil, false, nil
	}
	return out, true, nil
}

func diffSequence(desc *TypeDesc, base, upd reflect.Value, codecs *CodecRegistry, seen map[copyVisitKey]bool) (any, bool, error) {
	be, err := encodeValue(desc, base, codecs, seen)
	if err != nil {
		return nil, false, err
	}
	ue, err := encodeValue(desc, upd, codecs, seen)
	if err != nil {
		return nil, false, err
	}
	if reflect.DeepEqual(be, ue) {
		return nil, false, nil
	}
	return ue, true, nil
}

func diffMap(desc *TypeDesc, base, upd reflect.Value, codecs *CodecRegistry, seen map[copyVisitKey]bool) (any, bool, error) {
	bm := mapAsStringAny(base, codecs)
	um := mapAsStringAny(upd, codecs)
	if upd.IsNil() && base.IsNil() {
		return nil, false, nil
	}
	if upd.IsNil() && !base.IsNil() {
		return nil, true, nil
	}

	out := make(map[string]any)
	for k := range bm {
		if _, ok := um[k]; !ok {
			out[k] = nil
		}
	}
	for k, uv := range um {
		bv, ok := bm[k]
		if !ok {
			val, err := encodeValue(desc.Elem, uv, codecs, seen)
			if err != nil {
				return nil, false, err
			}
			out[k] = val
			continue
		}
		val, changed, err := diffValues(desc.Elem, bv, uv, codecs, seen)
		if err != nil {
			return nil, false, err
		}
		if changed {
			out[k] = val
		}
	}
	if len(out) == 0 {
		return nil, false, nil
	}
	return out, true, nil
}

func mapAsStringAny(mp reflect.Value, codecs *CodecRegistry) map[string]reflect.Value {
	out := make(map[string]reflect.Value)
	if !mp.IsValid() || mp.IsNil() {
		return out
	}
	iter := mp.MapRange()
	for iter.Next() {
		ks, err := codecs.Format(iter.Key())
		if err != nil {
			ks = fmt.Sprintf("%v", iter.Key().Interface())
		}
		out[ks] = iter.Value()
	}
	return out
}

func readFieldPath(v reflect.Value, path []int) (reflect.Value, bool) {
	cur := v
	for _, idx := range path {
		if !cur.IsValid() {
			return reflect.Value{}, false
		}
		if cur.Kind() == reflect.Pointer {
			if cur.IsNil() {
				return reflect.Value{}, false
			}
			cur = cur.Elem()
		}
		cur = cur.Field(idx)
	}
	return cur, true
}

func marshalIndented(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
