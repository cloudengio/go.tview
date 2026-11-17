// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package structmodel

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Validator may be implemented by configuration structs to provide custom
// validation rules checked during Document.Validate and Document.Commit.
type Validator interface {
	Validate() error
}

// DefaultInitializer may be implemented by structs to populate default values
// when a nil pointer is initialized or a new slice/map item is created.
type DefaultInitializer interface {
	SetDefaults()
}

// Option configures a Document.
type Option func(*docConfig)

type docConfig struct {
	codecs    *CodecRegistry
	rootLabel string
	validator func(any) error
}

// WithCodec registers a custom scalar Codec for type t.
func WithCodec(t reflect.Type, c Codec) Option {
	return func(cfg *docConfig) {
		if cfg.codecs == nil {
			cfg.codecs = NewCodecRegistry()
		}
		cfg.codecs.Register(t, c)
	}
}

// WithCodecs uses an existing CodecRegistry.
func WithCodecs(reg *CodecRegistry) Option {
	return func(cfg *docConfig) {
		cfg.codecs = reg
	}
}

// WithRootLabel sets the display label of the root node.
func WithRootLabel(label string) Option {
	return func(cfg *docConfig) {
		cfg.rootLabel = label
	}
}

// WithValidator registers an additional validation callback invoked with the
// working copy pointer during Validate and Commit.
func WithValidator(fn func(target any) error) Option {
	return func(cfg *docConfig) {
		cfg.validator = fn
	}
}

// Document manages a transactional editing session for a target struct pointer.
// Edits mutate an isolated working copy until Commit is called.
type Document struct {
	targetPtr  reflect.Value
	workingPtr reflect.Value
	schema     *Schema
	rootLabel  string
	validator  func(any) error
	dirty      bool
	fieldErrs  map[string]error
	root       *Node
}

// NewDocument creates a Document bound to target, which must be a non-nil
// pointer to a struct.
func NewDocument(target any, opts ...Option) (*Document, error) {
	if target == nil {
		return nil, errors.New("target must be a non-nil pointer to a struct")
	}
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("target must be a non-nil pointer to a struct, got %T", target)
	}

	cfg := &docConfig{
		codecs:    NewCodecRegistry(),
		rootLabel: rv.Elem().Type().Name(),
	}
	if cfg.rootLabel == "" {
		cfg.rootLabel = "config"
	}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	schema, err := BuildSchema(rv.Type(), cfg.codecs)
	if err != nil {
		return nil, err
	}

	working := DeepCopy(rv)
	return &Document{
		targetPtr:  rv,
		workingPtr: working,
		schema:     schema,
		rootLabel:  cfg.rootLabel,
		validator:  cfg.validator,
		fieldErrs:  make(map[string]error),
	}, nil
}

// Schema returns the compiled Schema for the document's root type.
func (d *Document) Schema() *Schema {
	return d.schema
}

// WorkingValue returns the working copy pointer passed to validators or
// callers inspecting staged edits.
func (d *Document) WorkingValue() any {
	return d.workingPtr.Interface()
}

// Dirty reports whether any edits have been made since creation or the last
// Commit/Revert.
func (d *Document) Dirty() bool {
	return d.dirty
}

// Root returns the root Node of the lazy configuration tree.
func (d *Document) Root() *Node {
	if d.root == nil {
		d.root = newRootNode(d)
	}
	return d.root
}

// Validate checks pending field parse/required errors, runs Validator.Validate
// on any structs in the working tree that implement it, and runs any custom
// document validator.
func (d *Document) Validate() error {
	var errs []error
	if len(d.fieldErrs) > 0 {
		keys := make([]string, 0, len(d.fieldErrs))
		for k := range d.fieldErrs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			errs = append(errs, fmt.Errorf("%s: %w", k, d.fieldErrs[k]))
		}
	}

	seen := make(map[copyVisitKey]bool)
	d.validateValue(d.workingPtr, "root", seen, &errs)

	if d.validator != nil {
		if err := d.validator(d.workingPtr.Interface()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Commit validates the working copy and copies it into the original target
// struct if valid.
func (d *Document) Commit() error {
	if err := d.Validate(); err != nil {
		return err
	}
	cloned := DeepCopy(d.workingPtr.Elem())
	d.targetPtr.Elem().Set(cloned)
	d.dirty = false
	return nil
}

// Revert discards all staged edits and restores the working copy from the
// original target struct.
func (d *Document) Revert() {
	d.workingPtr.Elem().Set(DeepCopy(d.targetPtr.Elem()))
	d.fieldErrs = make(map[string]error)
	d.dirty = false
	d.root = nil
}

func (d *Document) setFieldError(path string, err error) {
	if err == nil {
		delete(d.fieldErrs, path)
		return
	}
	d.fieldErrs[path] = err
}

func (d *Document) clearErrorsWithPrefix(prefix string) {
	dotPrefix := prefix + "."
	bracketPrefix := prefix + "["
	bracePrefix := prefix + "{"
	for k := range d.fieldErrs {
		if k == prefix || strings.HasPrefix(k, dotPrefix) ||
			strings.HasPrefix(k, bracketPrefix) || strings.HasPrefix(k, bracePrefix) {
			delete(d.fieldErrs, k)
		}
	}
}

func (d *Document) validateValue(v reflect.Value, path string, seen map[copyVisitKey]bool, errs *[]error) {
	if !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		key := copyVisitKey{ptr: v.Pointer(), typ: v.Type()}
		if seen[key] {
			return
		}
		seen[key] = true
		d.checkValidator(v, path, errs)
		d.validateValue(v.Elem(), path, seen, errs)
	case reflect.Struct:
		if _, ok := d.schema.Codecs.Classify(v.Type()); ok {
			return
		}
		if v.CanAddr() {
			d.checkValidator(v.Addr(), path, errs)
		} else {
			d.checkValidator(v, path, errs)
		}
		for i := 0; i < v.NumField(); i++ {
			sf := v.Type().Field(i)
			if !sf.IsExported() && !sf.Anonymous {
				continue
			}
			d.validateValue(v.Field(i), path+"."+sf.Name, seen, errs)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			d.validateValue(v.Index(i), fmt.Sprintf("%s[%d]", path, i), seen, errs)
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			kStr := fmt.Sprintf("%v", iter.Key().Interface())
			d.validateValue(iter.Value(), fmt.Sprintf("%s[%s]", path, kStr), seen, errs)
		}
	}
}

func (d *Document) checkValidator(v reflect.Value, path string, errs *[]error) {
	if !v.CanInterface() {
		return
	}
	if val, ok := v.Interface().(Validator); ok {
		if err := val.Validate(); err != nil {
			*errs = append(*errs, fmt.Errorf("%s: %w", path, err))
		}
	}
}
