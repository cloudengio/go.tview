// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package structmodel

import (
	"fmt"
	"reflect"
)

// ValueRef provides uniform read/write access to a Go value regardless of
// whether it lives in a struct field, slice element, pointer, or unaddressable
// map entry.
type ValueRef interface {
	Type() reflect.Type
	Get() reflect.Value
	Set(v reflect.Value) error
	DirectAddr() bool
}

type rootRef struct {
	val reflect.Value // addressable struct value
}

func newRootRef(ptr reflect.Value) ValueRef {
	return &rootRef{val: ptr.Elem()}
}

func (r *rootRef) Type() reflect.Type { return r.val.Type() }
func (r *rootRef) Get() reflect.Value { return r.val }
func (r *rootRef) DirectAddr() bool   { return true }
func (r *rootRef) Set(v reflect.Value) error {
	r.val.Set(v)
	return nil
}

type fieldRef struct {
	parent    ValueRef
	path      []int
	fieldType reflect.Type
}

func newFieldRef(parent ValueRef, path []int, ft reflect.Type) ValueRef {
	return &fieldRef{parent: parent, path: path, fieldType: ft}
}

func (f *fieldRef) Type() reflect.Type { return f.fieldType }
func (f *fieldRef) DirectAddr() bool   { return f.parent.DirectAddr() }

func (f *fieldRef) Get() reflect.Value {
	cur := f.parent.Get()
	for _, idx := range f.path {
		if !cur.IsValid() {
			return reflect.Zero(f.fieldType)
		}
		if cur.Kind() == reflect.Pointer {
			if cur.IsNil() {
				return reflect.Zero(f.fieldType)
			}
			cur = cur.Elem()
		}
		cur = cur.Field(idx)
	}
	return cur
}

func (f *fieldRef) Set(v reflect.Value) error {
	root := f.parent.Get()
	if !root.IsValid() {
		root = reflect.New(f.parent.Type()).Elem()
	}
	if !root.CanSet() {
		cp := reflect.New(root.Type()).Elem()
		cp.Set(root)
		root = cp
	}

	cur := root
	for i, idx := range f.path {
		if cur.Kind() == reflect.Pointer {
			if cur.IsNil() {
				alloc := reflect.New(cur.Type().Elem())
				cur.Set(alloc)
			}
			cur = cur.Elem()
		}
		if i == len(f.path)-1 {
			cur.Field(idx).Set(v)
			break
		}
		cur = cur.Field(idx)
	}

	if !f.parent.DirectAddr() {
		return f.parent.Set(root)
	}
	return nil
}

type sliceElemRef struct {
	parent   ValueRef
	index    int
	elemType reflect.Type
}

func newSliceElemRef(parent ValueRef, index int, elemType reflect.Type) ValueRef {
	return &sliceElemRef{parent: parent, index: index, elemType: elemType}
}

func (s *sliceElemRef) Type() reflect.Type { return s.elemType }
func (s *sliceElemRef) DirectAddr() bool   { return true }

func (s *sliceElemRef) Get() reflect.Value {
	sl := s.parent.Get()
	if !sl.IsValid() || s.index < 0 || s.index >= sl.Len() {
		return reflect.Zero(s.elemType)
	}
	return sl.Index(s.index)
}

func (s *sliceElemRef) Set(v reflect.Value) error {
	sl := s.parent.Get()
	if !sl.IsValid() || s.index < 0 || s.index >= sl.Len() {
		return fmt.Errorf("slice/array index %d out of bounds", s.index)
	}
	if sl.Index(s.index).CanSet() {
		sl.Index(s.index).Set(v)
		return nil
	}
	// Fixed arrays returned from a map entry are not addressable; copy and write back.
	cp := reflect.New(sl.Type()).Elem()
	cp.Set(sl)
	cp.Index(s.index).Set(v)
	return s.parent.Set(cp)
}

type mapEntryRef struct {
	parent   ValueRef
	key      reflect.Value
	elemType reflect.Type
}

func newMapEntryRef(parent ValueRef, key reflect.Value, elemType reflect.Type) ValueRef {
	return &mapEntryRef{parent: parent, key: key, elemType: elemType}
}

func (m *mapEntryRef) Type() reflect.Type { return m.elemType }
func (m *mapEntryRef) DirectAddr() bool   { return false }

func (m *mapEntryRef) Get() reflect.Value {
	mp := m.parent.Get()
	if !mp.IsValid() || mp.IsNil() {
		return reflect.Zero(m.elemType)
	}
	val := mp.MapIndex(m.key)
	if !val.IsValid() {
		return reflect.Zero(m.elemType)
	}
	cp := reflect.New(m.elemType).Elem()
	cp.Set(val)
	return cp
}

func (m *mapEntryRef) Set(v reflect.Value) error {
	mp := m.parent.Get()
	if !mp.IsValid() || mp.IsNil() {
		mp = reflect.MakeMap(m.parent.Type())
		mp.SetMapIndex(m.key, v)
		return m.parent.Set(mp)
	}
	mp.SetMapIndex(m.key, v)
	return nil
}

type ptrElemRef struct {
	parent   ValueRef
	elemType reflect.Type
}

func newPtrElemRef(parent ValueRef, elemType reflect.Type) ValueRef {
	return &ptrElemRef{parent: parent, elemType: elemType}
}

func (p *ptrElemRef) Type() reflect.Type { return p.elemType }
func (p *ptrElemRef) DirectAddr() bool   { return p.parent.DirectAddr() }

func (p *ptrElemRef) Get() reflect.Value {
	pv := p.parent.Get()
	if !pv.IsValid() || pv.IsNil() {
		return reflect.Zero(p.elemType)
	}
	return pv.Elem()
}

func (p *ptrElemRef) Set(v reflect.Value) error {
	pv := p.parent.Get()
	if !pv.IsValid() || pv.IsNil() {
		alloc := reflect.New(p.elemType)
		alloc.Elem().Set(v)
		return p.parent.Set(alloc)
	}
	pv.Elem().Set(v)
	if !p.parent.DirectAddr() {
		return p.parent.Set(pv)
	}
	return nil
}

type copyVisitKey struct {
	ptr uintptr
	typ reflect.Type
}

// DeepCopy returns a deep clone of src, guarding against pointer cycles.
func DeepCopy(src reflect.Value) reflect.Value {
	seen := make(map[copyVisitKey]reflect.Value)
	return deepCopyValue(src, seen)
}

func deepCopyValue(src reflect.Value, seen map[copyVisitKey]reflect.Value) reflect.Value {
	if !src.IsValid() {
		return src
	}
	t := src.Type()
	switch t.Kind() {
	case reflect.Pointer:
		return deepCopyPtr(src, t, seen)
	case reflect.Slice:
		return deepCopySlice(src, t, seen)
	case reflect.Array:
		dst := reflect.New(t).Elem()
		for i := 0; i < src.Len(); i++ {
			dst.Index(i).Set(deepCopyValue(src.Index(i), seen))
		}
		return dst
	case reflect.Map:
		return deepCopyMap(src, t, seen)
	case reflect.Struct:
		dst := reflect.New(t).Elem()
		// Preserve unexported fields (e.g. time.Time, netip.Addr).
		dst.Set(src)
		deepCopyStructFields(dst, src, seen)
		return dst
	default:
		dst := reflect.New(t).Elem()
		dst.Set(src)
		return dst
	}
}

func deepCopyStructFields(dst, src reflect.Value, seen map[copyVisitKey]reflect.Value) {
	t := src.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.IsExported() {
			dst.Field(i).Set(deepCopyValue(src.Field(i), seen))
			continue
		}
		if sf.Anonymous && sf.Type.Kind() == reflect.Struct {
			deepCopyStructFields(dst.Field(i), src.Field(i), seen)
		}
	}
}

func deepCopyPtr(src reflect.Value, t reflect.Type, seen map[copyVisitKey]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(t)
	}
	key := copyVisitKey{ptr: src.Pointer(), typ: t}
	if existing, ok := seen[key]; ok {
		return existing
	}
	dst := reflect.New(t.Elem())
	seen[key] = dst
	dst.Elem().Set(deepCopyValue(src.Elem(), seen))
	return dst
}

func deepCopySlice(src reflect.Value, t reflect.Type, seen map[copyVisitKey]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(t)
	}
	dst := reflect.MakeSlice(t, src.Len(), src.Len())
	for i := 0; i < src.Len(); i++ {
		dst.Index(i).Set(deepCopyValue(src.Index(i), seen))
	}
	return dst
}

func deepCopyMap(src reflect.Value, t reflect.Type, seen map[copyVisitKey]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(t)
	}
	dst := reflect.MakeMapWithSize(t, src.Len())
	iter := src.MapRange()
	for iter.Next() {
		k := deepCopyValue(iter.Key(), seen)
		v := deepCopyValue(iter.Value(), seen)
		dst.SetMapIndex(k, v)
	}
	return dst
}
