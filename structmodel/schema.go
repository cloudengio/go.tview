// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package structmodel

import (
	"fmt"
	"reflect"
	"sort"

	"cloudeng.io/tui/tview/structtag"
)

// Kind identifies the structural category of a TypeDesc.
type Kind int

const (
	KindScalar Kind = iota + 1
	KindStruct
	KindSlice
	KindArray
	KindMap
	KindPtr
)

// TypeDesc describes a Go type in the schema graph. Recursive types form cycles
// via Elem or FieldDesc.Type pointers.
type TypeDesc struct {
	Kind        Kind
	Type        reflect.Type
	ScalarKind  ScalarKind
	EnumOptions []string     // populated for enum KindScalar types
	Fields      []*FieldDesc // populated for KindStruct
	Key         *TypeDesc    // populated for KindMap
	Elem        *TypeDesc    // populated for KindSlice, KindArray, KindMap, KindPtr
}

// FieldDesc describes a single visible field on a KindStruct TypeDesc.
type FieldDesc struct {
	Tag               structtag.Spec
	Path              []int
	Type              *TypeDesc
	DeclIndex         int
	InlineStringSlice bool
}

// Schema holds the root TypeDesc and the CodecRegistry used to construct it.
type Schema struct {
	Root   *TypeDesc
	Codecs *CodecRegistry
}

// BuildSchema reflects on t (which may be a struct or pointer to struct) and
// builds a cyclic-safe Schema using codecs.
func BuildSchema(t reflect.Type, codecs *CodecRegistry) (*Schema, error) {
	if codecs == nil {
		codecs = NewCodecRegistry()
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct or pointer to struct, got %s", t)
	}
	b := &schemaBuilder{
		codecs: codecs,
		types:  make(map[reflect.Type]*TypeDesc),
	}
	root, err := b.buildType(t)
	if err != nil {
		return nil, err
	}
	return &Schema{Root: root, Codecs: codecs}, nil
}

type schemaBuilder struct {
	codecs *CodecRegistry
	types  map[reflect.Type]*TypeDesc
}

func (b *schemaBuilder) buildType(t reflect.Type) (*TypeDesc, error) {
	if cached, ok := b.types[t]; ok {
		return cached, nil
	}

	if sk, ok := b.codecs.Classify(t); ok {
		enumOpts, _ := ExtractEnumOptions(t)
		desc := &TypeDesc{
			Kind:        KindScalar,
			Type:        t,
			ScalarKind:  sk,
			EnumOptions: enumOpts,
		}
		b.types[t] = desc
		return desc, nil
	}

	desc := &TypeDesc{Type: t}
	// Register before recursing so recursive structs/slices/maps terminate.
	b.types[t] = desc

	var err error
	switch t.Kind() {
	case reflect.Pointer:
		desc.Kind = KindPtr
		desc.Elem, err = b.buildType(t.Elem())
	case reflect.Slice:
		desc.Kind = KindSlice
		desc.Elem, err = b.buildType(t.Elem())
	case reflect.Array:
		desc.Kind = KindArray
		desc.Elem, err = b.buildType(t.Elem())
	case reflect.Map:
		err = b.buildMap(t, desc)
	case reflect.Struct:
		desc.Kind = KindStruct
		desc.Fields, err = b.buildStructFields(t, map[reflect.Type]bool{t: true})
	default:
		err = fmt.Errorf("unsupported type %s (%s)", t, t.Kind())
	}
	if err != nil {
		delete(b.types, t)
		return nil, err
	}
	return desc, nil
}

func (b *schemaBuilder) buildMap(t reflect.Type, desc *TypeDesc) error {
	desc.Kind = KindMap
	keyDesc, err := b.buildType(t.Key())
	if err != nil {
		return err
	}
	if keyDesc.Kind != KindScalar {
		return fmt.Errorf("map %s has non-scalar key type %s", t, t.Key())
	}
	elemDesc, err := b.buildType(t.Elem())
	if err != nil {
		return err
	}
	desc.Key = keyDesc
	desc.Elem = elemDesc
	return nil
}

func (b *schemaBuilder) buildStructFields(t reflect.Type, inlineStack map[reflect.Type]bool) ([]*FieldDesc, error) {
	var fields []*FieldDesc
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() && !sf.Anonymous {
			continue
		}
		spec, err := structtag.Parse(sf)
		if err != nil {
			return nil, err
		}
		if !sf.IsExported() && !spec.Inline {
			continue
		}
		if spec.Hidden {
			continue
		}

		if spec.Inline {
			inlined, err := b.expandInlineField(sf, spec, inlineStack)
			if err != nil {
				return nil, err
			}
			for _, f := range inlined {
				f.DeclIndex = len(fields)
				fields = append(fields, f)
			}
			continue
		}

		ft, err := b.buildType(sf.Type)
		if err != nil {
			return nil, fmt.Errorf("field %s.%s: %w", t.Name(), sf.Name, err)
		}
		fd := &FieldDesc{
			Tag:               spec,
			Path:              []int{i},
			Type:              ft,
			DeclIndex:         len(fields),
			InlineStringSlice: isInlineStringSlice(sf.Type, spec),
		}
		fields = append(fields, fd)
	}

	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].Tag.Order != fields[j].Tag.Order {
			return fields[i].Tag.Order < fields[j].Tag.Order
		}
		return fields[i].DeclIndex < fields[j].DeclIndex
	})
	return fields, nil
}

func (b *schemaBuilder) expandInlineField(sf reflect.StructField, parentSpec structtag.Spec, inlineStack map[reflect.Type]bool) ([]*FieldDesc, error) {
	ft := sf.Type
	if ft.Kind() == reflect.Pointer {
		ft = ft.Elem()
	}
	if ft.Kind() != reflect.Struct {
		return nil, fmt.Errorf("field %s: inline tag requires struct or *struct, got %s", sf.Name, sf.Type)
	}
	if inlineStack[ft] {
		return nil, fmt.Errorf("field %s: cyclic inline struct %s", sf.Name, ft)
	}

	inlineStack[ft] = true
	defer delete(inlineStack, ft)

	subFields, err := b.buildStructFields(ft, inlineStack)
	if err != nil {
		return nil, err
	}

	out := make([]*FieldDesc, 0, len(subFields))
	for _, sub := range subFields {
		path := make([]int, 0, 1+len(sub.Path))
		path = append(path, sf.Index[0])
		path = append(path, sub.Path...)
		tag := sub.Tag
		if parentSpec.ReadOnly {
			tag.ReadOnly = true
		}
		out = append(out, &FieldDesc{
			Tag:               tag,
			Path:              path,
			Type:              sub.Type,
			InlineStringSlice: sub.InlineStringSlice,
		})
	}
	return out, nil
}

func isInlineStringSlice(t reflect.Type, spec structtag.Spec) bool {
	if t.Kind() != reflect.Slice || t.Elem().Kind() != reflect.String {
		return false
	}
	return spec.Widget == "csv" || spec.Widget == "lines"
}
