// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package structmodel

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"sort"
	"strings"

	"cloudeng.io/tui/tview/structtag"
)

// Node represents an addressable structural item (struct, pointer, slice,
// array, or map) in the lazy configuration tree.
type Node struct {
	doc       *Document
	path      string
	label     string
	tag       structtag.Spec
	desc      *TypeDesc
	ref       ValueRef
	readOnly  bool
	ancestors map[copyVisitKey]bool
	hasCycle  bool
}

func newRootNode(doc *Document) *Node {
	anc := make(map[copyVisitKey]bool)
	ptr := doc.workingPtr
	if ptr.IsValid() && !ptr.IsNil() {
		anc[copyVisitKey{ptr: ptr.Pointer(), typ: ptr.Type()}] = true
	}
	return &Node{
		doc:       doc,
		path:      doc.rootLabel,
		label:     doc.rootLabel,
		tag:       structtag.Spec{Name: doc.rootLabel, Label: doc.rootLabel, Expand: true},
		desc:      doc.schema.Root,
		ref:       newRootRef(doc.workingPtr),
		ancestors: anc,
	}
}

func newChildNode(parent *Node, path, label string, tag structtag.Spec, desc *TypeDesc, ref ValueRef, readOnly bool) *Node {
	anc := maps.Clone(parent.ancestors)
	if anc == nil {
		anc = make(map[copyVisitKey]bool)
	}
	cycle := false
	if desc.Kind == KindPtr {
		v := ref.Get()
		if v.IsValid() && !v.IsNil() {
			key := copyVisitKey{ptr: v.Pointer(), typ: v.Type()}
			if anc[key] {
				cycle = true
			} else {
				anc[key] = true
			}
		}
	}
	return &Node{
		doc:       parent.doc,
		path:      path,
		label:     label,
		tag:       tag,
		desc:      desc,
		ref:       ref,
		readOnly:  readOnly || parent.readOnly || tag.ReadOnly,
		ancestors: anc,
		hasCycle:  cycle,
	}
}

// Path returns the dot/bracket path to this node.
func (n *Node) Path() string { return n.path }

// Label returns the display label for this node.
func (n *Node) Label() string { return n.label }

// Tag returns the merged structtag.Spec for the field represented by this node.
func (n *Node) Tag() structtag.Spec { return n.tag }

// ReadOnly reports whether edits on this node are disabled.
func (n *Node) ReadOnly() bool { return n.readOnly }

// HasCycle reports whether this node points to an already-visited ancestor
// pointer in memory, preventing infinite tree expansion.
func (n *Node) HasCycle() bool { return n.hasCycle }

// IsPointer reports whether the underlying Go type is a pointer.
func (n *Node) IsPointer() bool { return n.desc.Kind == KindPtr }

// IsNil reports whether the underlying pointer, slice, or map is nil.
func (n *Node) IsNil() bool {
	v := n.ref.Get()
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map:
		return v.IsNil()
	default:
		return false
	}
}

// TargetKind returns the underlying non-pointer Kind of this node.
func (n *Node) TargetKind() Kind {
	d := n.effectiveDesc()
	return d.Kind
}

// TargetTypeName returns a human-readable type name for the target value.
func (n *Node) TargetTypeName() string {
	return n.effectiveDesc().Type.String()
}

func (n *Node) effectiveDesc() *TypeDesc {
	d := n.desc
	for d.Kind == KindPtr && d.Elem != nil {
		d = d.Elem
	}
	return d
}

func (n *Node) effectiveRef() ValueRef {
	ref := n.ref
	d := n.desc
	for d.Kind == KindPtr && d.Elem != nil {
		v := ref.Get()
		if !v.IsValid() || v.IsNil() {
			return nil
		}
		ref = newPtrElemRef(ref, d.Elem.Type)
		d = d.Elem
	}
	return ref
}

// Summary returns a concise status suffix for display in trees and tables.
func (n *Node) Summary() string {
	if n.hasCycle {
		return "[cycle]"
	}
	if n.IsPointer() && n.IsNil() {
		return "[nil]"
	}
	ref := n.effectiveRef()
	if ref == nil {
		return "[nil]"
	}
	d := n.effectiveDesc()
	v := ref.Get()
	switch d.Kind {
	case KindStruct:
		if n.tag.ItemLabel != "" {
			if lbl, ok := readItemLabel(v, n.tag.ItemLabel); ok {
				return lbl
			}
		}
		return fmt.Sprintf("{%d}", len(d.Fields))
	case KindSlice, KindArray:
		return fmt.Sprintf("[%d]", v.Len())
	case KindMap:
		return fmt.Sprintf("{%d}", v.Len())
	case KindScalar:
		s, err := n.doc.schema.Codecs.Format(v)
		if err != nil {
			return "<err>"
		}
		return s
	default:
		return ""
	}
}

// InitPtr allocates a zero-initialized value (calling DefaultInitializer if
// implemented) when this node is a nil pointer.
func (n *Node) InitPtr() error {
	if n.readOnly {
		return errors.New("node is read-only")
	}
	if n.desc.Kind != KindPtr {
		return errors.New("node is not a pointer")
	}
	alloc := newDefaultValue(n.desc.Elem.Type, false)
	ptr := reflect.New(n.desc.Elem.Type)
	ptr.Elem().Set(alloc)
	if err := n.ref.Set(ptr); err != nil {
		return err
	}
	n.doc.dirty = true
	return nil
}

// ClearPtr resets a pointer node to nil and clears any nested field errors.
func (n *Node) ClearPtr() error {
	if n.readOnly {
		return errors.New("node is read-only")
	}
	if n.desc.Kind != KindPtr {
		return errors.New("node is not a pointer")
	}
	if err := n.ref.Set(reflect.Zero(n.desc.Type)); err != nil {
		return err
	}
	n.doc.clearErrorsWithPrefix(n.path)
	n.doc.dirty = true
	return nil
}

// ScalarFields returns the scalar (and inline-string-slice) bindings editable
// directly in a form for this node.
func (n *Node) ScalarFields() []*FieldBinding {
	if n.hasCycle || (n.IsPointer() && n.IsNil()) {
		return nil
	}
	ref := n.effectiveRef()
	if ref == nil {
		return nil
	}
	d := n.effectiveDesc()
	if d.Kind == KindScalar {
		tag := applyEnumOptions(n.tag, d)
		return []*FieldBinding{{
			doc:        n.doc,
			id:         n.path,
			tag:        tag,
			desc:       d,
			ref:        ref,
			readOnly:   n.readOnly,
			scalarKind: d.ScalarKind,
		}}
	}
	if d.Kind != KindStruct {
		return nil
	}
	var out []*FieldBinding
	for _, f := range d.Fields {
		if f.Type.Kind != KindScalar && !f.InlineStringSlice {
			continue
		}
		tag := applyEnumOptions(f.Tag, f.Type)
		out = append(out, &FieldBinding{
			doc:               n.doc,
			id:                n.path + "." + f.Tag.Name,
			tag:               tag,
			desc:              f.Type,
			ref:               newFieldRef(ref, f.Path, f.Type.Type),
			readOnly:          n.readOnly || f.Tag.ReadOnly,
			scalarKind:        f.Type.ScalarKind,
			inlineStringSlice: f.InlineStringSlice,
		})
	}
	return out
}

func applyEnumOptions(tag structtag.Spec, desc *TypeDesc) structtag.Spec {
	if len(tag.Options) == 0 && len(desc.EnumOptions) > 0 {
		tag.Options = desc.EnumOptions
	}
	return tag
}

// Children lazily returns structural child nodes (nested structs, pointers,
// slices, arrays, and maps) for tree navigation.
func (n *Node) Children() []*Node {
	if n.hasCycle || (n.IsPointer() && n.IsNil()) {
		return nil
	}
	ref := n.effectiveRef()
	if ref == nil {
		return nil
	}
	d := n.effectiveDesc()
	switch d.Kind {
	case KindStruct:
		return n.structChildren(ref, d)
	case KindSlice, KindArray:
		return n.sliceChildren(ref, d)
	case KindMap:
		return n.mapChildren(ref, d)
	default:
		return nil
	}
}

func (n *Node) structChildren(ref ValueRef, d *TypeDesc) []*Node {
	var out []*Node
	for _, f := range d.Fields {
		if f.Type.Kind == KindScalar || f.InlineStringSlice {
			continue
		}
		childPath := n.path + "." + f.Tag.Name
		childRef := newFieldRef(ref, f.Path, f.Type.Type)
		out = append(out, newChildNode(n, childPath, f.Tag.Label, f.Tag, f.Type, childRef, f.Tag.ReadOnly))
	}
	return out
}

func (n *Node) sliceChildren(ref ValueRef, d *TypeDesc) []*Node {
	if !isContainerOrPtr(d.Elem) {
		return nil
	}
	v := ref.Get()
	out := make([]*Node, 0, v.Len())
	for i := 0; i < v.Len(); i++ {
		itemRef := newSliceElemRef(ref, i, d.Elem.Type)
		lbl := n.formatSliceItemLabel(i, itemRef.Get())
		itemTag := structtag.Spec{
			Name:      fmt.Sprintf("[%d]", i),
			Label:     lbl,
			ItemLabel: n.tag.ItemLabel,
		}
		out = append(out, newChildNode(n, fmt.Sprintf("%s[%d]", n.path, i), lbl, itemTag, d.Elem, itemRef, n.readOnly))
	}
	return out
}

func (n *Node) mapChildren(ref ValueRef, d *TypeDesc) []*Node {
	if !isContainerOrPtr(d.Elem) {
		return nil
	}
	entries := n.sortedMapKeys(ref)
	out := make([]*Node, 0, len(entries))
	for _, e := range entries {
		entryRef := newMapEntryRef(ref, e.rawKey, d.Elem.Type)
		entryTag := structtag.Spec{
			Name:      e.formatted,
			Label:     e.formatted,
			ItemLabel: n.tag.ItemLabel,
		}
		out = append(out, newChildNode(n, fmt.Sprintf("%s[%s]", n.path, e.formatted), e.formatted, entryTag, d.Elem, entryRef, n.readOnly))
	}
	return out
}

func isContainerOrPtr(d *TypeDesc) bool {
	return d.Kind != KindScalar
}

// Len returns the number of elements in a slice, array, or map node.
func (n *Node) Len() int {
	ref := n.effectiveRef()
	if ref == nil {
		return 0
	}
	v := ref.Get()
	switch v.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		return v.Len()
	default:
		return 0
	}
}

// IsFixedArray reports whether this node is a fixed-length Go array.
func (n *Node) IsFixedArray() bool {
	return n.effectiveDesc().Kind == KindArray
}

// ElemIsScalar reports whether slice/array/map elements are scalars.
func (n *Node) ElemIsScalar() bool {
	d := n.effectiveDesc()
	return d.Elem != nil && d.Elem.Kind == KindScalar
}

// ItemSummary returns a short summary for element i of a slice or array.
func (n *Node) ItemSummary(i int) string {
	ref := n.effectiveRef()
	if ref == nil {
		return ""
	}
	v := ref.Get()
	if i < 0 || i >= v.Len() {
		return ""
	}
	return n.summarizeValue(v.Index(i))
}

// ItemScalar formats scalar element i of a slice or array.
func (n *Node) ItemScalar(i int) (string, error) {
	ref := n.effectiveRef()
	if ref == nil {
		return "", errors.New("nil container")
	}
	v := ref.Get()
	if i < 0 || i >= v.Len() {
		return "", fmt.Errorf("index %d out of range", i)
	}
	return n.doc.schema.Codecs.Format(v.Index(i))
}

// SetItemScalar parses text and stores it at element i of a scalar slice/array.
func (n *Node) SetItemScalar(i int, text string) error {
	if n.readOnly {
		return errors.New("node is read-only")
	}
	ref := n.effectiveRef()
	if ref == nil {
		return errors.New("nil container")
	}
	d := n.effectiveDesc()
	parsed, err := n.doc.schema.Codecs.Parse(d.Elem.Type, text)
	if err != nil {
		return err
	}
	elemRef := newSliceElemRef(ref, i, d.Elem.Type)
	if err := elemRef.Set(parsed); err != nil {
		return err
	}
	n.doc.dirty = true
	return nil
}

// AppendItem appends a new default-initialized element to a slice node and
// returns its index.
func (n *Node) AppendItem() (int, error) {
	if n.readOnly {
		return -1, errors.New("node is read-only")
	}
	d := n.effectiveDesc()
	if d.Kind != KindSlice {
		return -1, errors.New("append requires a slice")
	}
	ref := n.effectiveRef()
	if ref == nil {
		return -1, errors.New("nil pointer container")
	}
	sl := ref.Get()
	if sl.IsNil() {
		sl = reflect.MakeSlice(d.Type, 0, 1)
	}
	item := newDefaultValue(d.Elem.Type, false)
	sl = reflect.Append(sl, item)
	if err := ref.Set(sl); err != nil {
		return -1, err
	}
	n.doc.dirty = true
	return sl.Len() - 1, nil
}

// DuplicateItem clones element i and appends it to a slice node.
func (n *Node) DuplicateItem(i int) (int, error) {
	if n.readOnly {
		return -1, errors.New("node is read-only")
	}
	d := n.effectiveDesc()
	if d.Kind != KindSlice {
		return -1, errors.New("duplicate requires a slice")
	}
	ref := n.effectiveRef()
	if ref == nil {
		return -1, errors.New("nil pointer container")
	}
	sl := ref.Get()
	if i < 0 || i >= sl.Len() {
		return -1, fmt.Errorf("index %d out of range", i)
	}
	cloned := DeepCopy(sl.Index(i))
	sl = reflect.Append(sl, cloned)
	if err := ref.Set(sl); err != nil {
		return -1, err
	}
	n.doc.dirty = true
	return sl.Len() - 1, nil
}

// DeleteItem removes element i from a slice node.
func (n *Node) DeleteItem(i int) error {
	if n.readOnly {
		return errors.New("node is read-only")
	}
	d := n.effectiveDesc()
	if d.Kind != KindSlice {
		return errors.New("delete requires a slice")
	}
	ref := n.effectiveRef()
	if ref == nil {
		return errors.New("nil pointer container")
	}
	sl := ref.Get()
	if i < 0 || i >= sl.Len() {
		return fmt.Errorf("index %d out of range", i)
	}
	next := reflect.MakeSlice(d.Type, 0, sl.Len()-1)
	for idx := 0; idx < sl.Len(); idx++ {
		if idx != i {
			next = reflect.Append(next, sl.Index(idx))
		}
	}
	if err := ref.Set(next); err != nil {
		return err
	}
	n.doc.clearErrorsWithPrefix(n.path)
	n.doc.dirty = true
	return nil
}

// MoveItem swaps or shifts an element from index from to index to in a slice or array.
func (n *Node) MoveItem(from, to int) error {
	if n.readOnly {
		return errors.New("node is read-only")
	}
	d := n.effectiveDesc()
	if d.Kind != KindSlice && d.Kind != KindArray {
		return errors.New("move requires a slice or array")
	}
	ref := n.effectiveRef()
	if ref == nil {
		return errors.New("nil pointer container")
	}
	sl := ref.Get()
	if from < 0 || from >= sl.Len() || to < 0 || to >= sl.Len() {
		return fmt.Errorf("move indices (%d, %d) out of range", from, to)
	}
	if from == to {
		return nil
	}
	a := DeepCopy(sl.Index(from))
	b := DeepCopy(sl.Index(to))
	if err := newSliceElemRef(ref, from, d.Elem.Type).Set(b); err != nil {
		return err
	}
	if err := newSliceElemRef(ref, to, d.Elem.Type).Set(a); err != nil {
		return err
	}
	n.doc.dirty = true
	return nil
}

type mapKeyItem struct {
	formatted string
	rawKey    reflect.Value
}

func (n *Node) sortedMapKeys(ref ValueRef) []mapKeyItem {
	mp := ref.Get()
	if !mp.IsValid() || mp.IsNil() {
		return nil
	}
	items := make([]mapKeyItem, 0, mp.Len())
	iter := mp.MapRange()
	for iter.Next() {
		k := iter.Key()
		s, err := n.doc.schema.Codecs.Format(k)
		if err != nil {
			s = fmt.Sprintf("%v", k.Interface())
		}
		items = append(items, mapKeyItem{formatted: s, rawKey: k})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].formatted < items[j].formatted
	})
	return items
}

// MapKeys returns sorted formatted keys of a map node.
func (n *Node) MapKeys() []string {
	ref := n.effectiveRef()
	if ref == nil {
		return nil
	}
	items := n.sortedMapKeys(ref)
	keys := make([]string, len(items))
	for i, it := range items {
		keys[i] = it.formatted
	}
	return keys
}

// MapEntrySummary returns a concise summary for the map value at key.
func (n *Node) MapEntrySummary(key string) string {
	ref := n.effectiveRef()
	if ref == nil {
		return ""
	}
	d := n.effectiveDesc()
	rawKey, err := n.doc.schema.Codecs.Parse(d.Key.Type, key)
	if err != nil {
		return ""
	}
	mp := ref.Get()
	if !mp.IsValid() || mp.IsNil() {
		return ""
	}
	val := mp.MapIndex(rawKey)
	if !val.IsValid() {
		return ""
	}
	return n.summarizeValue(val)
}

// AddMapEntry inserts a new entry under key with a default-initialized value.
func (n *Node) AddMapEntry(key string) error {
	if n.readOnly {
		return errors.New("node is read-only")
	}
	d := n.effectiveDesc()
	if d.Kind != KindMap {
		return errors.New("node is not a map")
	}
	rawKey, err := n.doc.schema.Codecs.Parse(d.Key.Type, key)
	if err != nil {
		return fmt.Errorf("invalid key %q: %w", key, err)
	}
	ref := n.effectiveRef()
	if ref == nil {
		return errors.New("nil pointer container")
	}
	mp := ref.Get()
	if mp.IsValid() && !mp.IsNil() && mp.MapIndex(rawKey).IsValid() {
		return fmt.Errorf("key %q already exists", key)
	}
	val := newDefaultValue(d.Elem.Type, false)
	if err := newMapEntryRef(ref, rawKey, d.Elem.Type).Set(val); err != nil {
		return err
	}
	n.doc.dirty = true
	return nil
}

// RenameMapKey renames an existing map entry from oldKey to newKey.
func (n *Node) RenameMapKey(oldKey, newKey string) error {
	if n.readOnly {
		return errors.New("node is read-only")
	}
	d := n.effectiveDesc()
	if d.Kind != KindMap {
		return errors.New("node is not a map")
	}
	rkOld, err := n.doc.schema.Codecs.Parse(d.Key.Type, oldKey)
	if err != nil {
		return fmt.Errorf("invalid key %q: %w", oldKey, err)
	}
	rkNew, err := n.doc.schema.Codecs.Parse(d.Key.Type, newKey)
	if err != nil {
		return fmt.Errorf("invalid key %q: %w", newKey, err)
	}
	ref := n.effectiveRef()
	if ref == nil {
		return errors.New("nil pointer container")
	}
	mp := ref.Get()
	if !mp.IsValid() || mp.IsNil() {
		return fmt.Errorf("key %q not found", oldKey)
	}
	existing := mp.MapIndex(rkOld)
	if !existing.IsValid() {
		return fmt.Errorf("key %q not found", oldKey)
	}
	if oldKey == newKey {
		return nil
	}
	if mp.MapIndex(rkNew).IsValid() {
		return fmt.Errorf("key %q already exists", newKey)
	}
	val := DeepCopy(existing)
	mp.SetMapIndex(rkOld, reflect.Value{})
	mp.SetMapIndex(rkNew, val)
	n.doc.clearErrorsWithPrefix(fmt.Sprintf("%s[%s]", n.path, oldKey))
	n.doc.dirty = true
	return nil
}

// DeleteMapEntry removes key from a map node.
func (n *Node) DeleteMapEntry(key string) error {
	if n.readOnly {
		return errors.New("node is read-only")
	}
	d := n.effectiveDesc()
	if d.Kind != KindMap {
		return errors.New("node is not a map")
	}
	rawKey, err := n.doc.schema.Codecs.Parse(d.Key.Type, key)
	if err != nil {
		return fmt.Errorf("invalid key %q: %w", key, err)
	}
	ref := n.effectiveRef()
	if ref == nil {
		return errors.New("nil pointer container")
	}
	mp := ref.Get()
	if !mp.IsValid() || mp.IsNil() || !mp.MapIndex(rawKey).IsValid() {
		return fmt.Errorf("key %q not found", key)
	}
	mp.SetMapIndex(rawKey, reflect.Value{})
	n.doc.clearErrorsWithPrefix(fmt.Sprintf("%s[%s]", n.path, key))
	n.doc.dirty = true
	return nil
}

// SetMapEntryScalar parses text and assigns it to a scalar map entry.
func (n *Node) SetMapEntryScalar(key, text string) error {
	if n.readOnly {
		return errors.New("node is read-only")
	}
	d := n.effectiveDesc()
	if d.Kind != KindMap || d.Elem.Kind != KindScalar {
		return errors.New("node is not a scalar map")
	}
	rawKey, err := n.doc.schema.Codecs.Parse(d.Key.Type, key)
	if err != nil {
		return fmt.Errorf("invalid key %q: %w", key, err)
	}
	parsed, err := n.doc.schema.Codecs.Parse(d.Elem.Type, text)
	if err != nil {
		return err
	}
	ref := n.effectiveRef()
	if ref == nil {
		return errors.New("nil pointer container")
	}
	if err := newMapEntryRef(ref, rawKey, d.Elem.Type).Set(parsed); err != nil {
		return err
	}
	n.doc.dirty = true
	return nil
}

func (n *Node) formatSliceItemLabel(i int, v reflect.Value) string {
	if n.tag.ItemLabel != "" {
		if lbl, ok := readItemLabel(v, n.tag.ItemLabel); ok && lbl != "" {
			return fmt.Sprintf("[%d] %s", i, lbl)
		}
	}
	return fmt.Sprintf("[%d]", i)
}

func (n *Node) summarizeValue(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return "[nil]"
		}
		v = v.Elem()
	}
	if _, ok := n.doc.schema.Codecs.Classify(v.Type()); ok {
		s, err := n.doc.schema.Codecs.Format(v)
		if err != nil {
			return "<err>"
		}
		return s
	}
	switch v.Kind() {
	case reflect.Struct:
		if n.tag.ItemLabel != "" {
			if lbl, ok := readItemLabel(v, n.tag.ItemLabel); ok && lbl != "" {
				return lbl
			}
		}
		return summarizeStruct(v, n.doc.schema.Codecs)
	case reflect.Slice, reflect.Array:
		return fmt.Sprintf("[%d items]", v.Len())
	case reflect.Map:
		return fmt.Sprintf("{%d entries}", v.Len())
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}

func summarizeStruct(v reflect.Value, codecs *CodecRegistry) string {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		if _, ok := codecs.Classify(sf.Type); ok {
			s, err := codecs.Format(v.Field(i))
			if err == nil && s != "" {
				return fmt.Sprintf("%s=%s", sf.Name, s)
			}
		}
	}
	return fmt.Sprintf("{%s}", t.Name())
}

func readItemLabel(v reflect.Value, fieldName string) (string, bool) {
	if !v.IsValid() {
		return "", false
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return "[nil]", true
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return "", false
	}
	fv := v.FieldByName(fieldName)
	if !fv.IsValid() {
		return "", false
	}
	return fmt.Sprintf("%v", fv.Interface()), true
}

func newDefaultValue(t reflect.Type, allocPtr bool) reflect.Value {
	if t.Kind() == reflect.Pointer {
		if !allocPtr {
			return reflect.Zero(t)
		}
		ptr := reflect.New(t.Elem())
		ptr.Elem().Set(newDefaultValue(t.Elem(), false))
		return ptr
	}
	if t.Kind() == reflect.Map {
		return reflect.MakeMap(t)
	}
	v := reflect.New(t)
	if init, ok := v.Interface().(DefaultInitializer); ok {
		init.SetDefaults()
	}
	return v.Elem()
}

// FieldBinding connects a scalar (or inline string slice) form field to its
// underlying ValueRef in the Document.
type FieldBinding struct {
	doc               *Document
	id                string
	tag               structtag.Spec
	desc              *TypeDesc
	ref               ValueRef
	readOnly          bool
	scalarKind        ScalarKind
	inlineStringSlice bool
}

// ID returns the unique path identifier for this field.
func (f *FieldBinding) ID() string { return f.id }

// Tag returns the merged structtag.Spec for this field.
func (f *FieldBinding) Tag() structtag.Spec { return f.tag }

// Type returns the Go reflect.Type of this field.
func (f *FieldBinding) Type() reflect.Type { return f.desc.Type }

// ScalarKind returns the ScalarKind of this field.
func (f *FieldBinding) ScalarKind() ScalarKind { return f.scalarKind }

// ReadOnly reports whether editing this field is disabled.
func (f *FieldBinding) ReadOnly() bool { return f.readOnly }

// IsSliceString reports whether this field binds a []string via csv or lines.
func (f *FieldBinding) IsSliceString() bool { return f.inlineStringSlice }

// Error returns the last parse or validation error recorded for this field.
func (f *FieldBinding) Error() error { return f.doc.fieldErrs[f.id] }

// Bool returns the boolean value for ScalarBool fields.
func (f *FieldBinding) Bool() bool {
	v := f.ref.Get()
	if !v.IsValid() || v.Kind() != reflect.Bool {
		return false
	}
	return v.Bool()
}

// SetBool updates a ScalarBool field.
func (f *FieldBinding) SetBool(b bool) error {
	if f.readOnly {
		return errors.New("field is read-only")
	}
	v := reflect.New(f.desc.Type).Elem()
	v.SetBool(b)
	if err := f.ref.Set(v); err != nil {
		f.doc.setFieldError(f.id, err)
		return err
	}
	f.doc.setFieldError(f.id, nil)
	f.doc.dirty = true
	return nil
}

// String formats the current value as text for display in form inputs.
func (f *FieldBinding) String() string {
	v := f.ref.Get()
	if f.inlineStringSlice {
		if !v.IsValid() || v.IsNil() {
			return ""
		}
		items := make([]string, v.Len())
		for i := 0; i < v.Len(); i++ {
			items[i] = v.Index(i).String()
		}
		sep := ", "
		if f.tag.Widget == "lines" {
			sep = "\n"
		}
		return strings.Join(items, sep)
	}
	s, err := f.doc.schema.Codecs.Format(v)
	if err != nil {
		return ""
	}
	return s
}

// SetString parses text and assigns the value if valid, recording any parse or
// required-field error on the Document so Commit will reject invalid inputs.
func (f *FieldBinding) SetString(text string) error {
	if f.readOnly {
		return errors.New("field is read-only")
	}
	if f.inlineStringSlice {
		return f.setInlineStringSlice(text)
	}
	if f.tag.Required && strings.TrimSpace(text) == "" {
		err := errors.New("value is required")
		f.doc.setFieldError(f.id, err)
		return err
	}
	parsed, err := f.doc.schema.Codecs.Parse(f.desc.Type, text)
	if err != nil {
		f.doc.setFieldError(f.id, err)
		return err
	}
	if err := f.ref.Set(parsed); err != nil {
		f.doc.setFieldError(f.id, err)
		return err
	}
	f.doc.setFieldError(f.id, nil)
	f.doc.dirty = true
	return nil
}

func (f *FieldBinding) setInlineStringSlice(text string) error {
	var parts []string
	if strings.TrimSpace(text) != "" {
		sep := ","
		if f.tag.Widget == "lines" {
			sep = "\n"
		}
		for _, p := range strings.Split(text, sep) {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" || f.tag.Widget == "csv" {
				parts = append(parts, trimmed)
			}
		}
	}
	if f.tag.Required && len(parts) == 0 {
		err := errors.New("at least one item is required")
		f.doc.setFieldError(f.id, err)
		return err
	}
	sl := reflect.MakeSlice(f.desc.Type, len(parts), len(parts))
	for i, p := range parts {
		sl.Index(i).SetString(p)
	}
	if err := f.ref.Set(sl); err != nil {
		f.doc.setFieldError(f.id, err)
		return err
	}
	f.doc.setFieldError(f.id, nil)
	f.doc.dirty = true
	return nil
}
