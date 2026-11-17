// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

// Package structedit provides a split-view rivo/tview settings editor for
// arbitrary Go structs annotated with yaml and tview struct tags.
package structedit

import (
	"reflect"

	"cloudeng.io/tui/tview/structmodel"
	"github.com/rivo/tview"
)

// WidgetFactory constructs a custom tview.FormItem bound to field. Callers
// should invoke notifyChange whenever the widget mutates field.
type WidgetFactory func(field *structmodel.FieldBinding, notifyChange func()) tview.FormItem

// Option configures an Editor.
type Option func(*editorConfig)

type editorConfig struct {
	title       string
	readOnly    bool
	app         *tview.Application
	onSave      func(target any) error
	onCancel    func()
	onChange    func()
	codecs      *structmodel.CodecRegistry
	namedWidget map[string]WidgetFactory
	typeWidget  map[reflect.Type]WidgetFactory
	validator   func(any) error
}

func defaultConfig() *editorConfig {
	return &editorConfig{
		title:       "Settings",
		codecs:      structmodel.NewCodecRegistry(),
		namedWidget: make(map[string]WidgetFactory),
		typeWidget:  make(map[reflect.Type]WidgetFactory),
	}
}

// WithTitle sets the title shown in the editor header bar.
func WithTitle(title string) Option {
	return func(c *editorConfig) {
		c.title = title
	}
}

// WithReadOnly disables all edits across the editor.
func WithReadOnly(readOnly bool) Option {
	return func(c *editorConfig) {
		c.readOnly = readOnly
	}
}

// WithApp attaches a tview.Application for explicit focus updates when
// switching panes or dismissing modals.
func WithApp(app *tview.Application) Option {
	return func(c *editorConfig) {
		c.app = app
	}
}

// WithOnSave sets a callback invoked after Document.Commit succeeds.
func WithOnSave(fn func(target any) error) Option {
	return func(c *editorConfig) {
		c.onSave = fn
	}
}

// WithOnCancel sets a callback invoked when the user cancels editing.
func WithOnCancel(fn func()) Option {
	return func(c *editorConfig) {
		c.onCancel = fn
	}
}

// WithOnChange sets a callback invoked whenever a field or collection is edited.
func WithOnChange(fn func()) Option {
	return func(c *editorConfig) {
		c.onChange = fn
	}
}

// WithCodec registers a custom scalar Codec for type t.
func WithCodec(t reflect.Type, codec structmodel.Codec) Option {
	return func(c *editorConfig) {
		c.codecs.Register(t, codec)
	}
}

// WithWidget registers a custom WidgetFactory invoked when a field specifies
// tview:"widget=<name>".
func WithWidget(name string, factory WidgetFactory) Option {
	return func(c *editorConfig) {
		c.namedWidget[name] = factory
	}
}

// WithTypeWidget registers a custom WidgetFactory for fields of type t.
func WithTypeWidget(t reflect.Type, factory WidgetFactory) Option {
	return func(c *editorConfig) {
		c.typeWidget[t] = factory
	}
}

// WithValidator registers a document-level validation function checked on Save.
func WithValidator(fn func(target any) error) Option {
	return func(c *editorConfig) {
		c.validator = fn
	}
}
