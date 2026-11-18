// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

// Package structtag parses yaml and tview struct tags into unified field metadata
// for UI generation.
package structtag

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// MaskMode controls whether and how a field's value is masked in the UI.
type MaskMode int

const (
	// MaskNone displays the field value normally.
	MaskNone MaskMode = iota
	// MaskAlwaysHide always displays a fixed number of asterisks regardless of
	// value length and provides no reveal toggle.
	MaskAlwaysHide
	// MaskHideWithShow displays asterisks by default with an option to reveal
	// the underlying value.
	MaskHideWithShow
)

// Spec describes the resolved metadata for a struct field after merging its
// yaml, help/doc, and tview struct tags.
type Spec struct {
	// FieldName is the Go struct field name.
	FieldName string
	// YAMLName is the field name derived from the yaml tag (empty if absent or "-").
	YAMLName string
	// Name is the effective serialized key name (YAMLName if set, otherwise FieldName).
	Name string
	// Label is the display label shown in the UI (tview label if set, otherwise Name).
	Label string
	// Hidden indicates whether the field should be omitted from the UI.
	Hidden bool
	// Inline indicates that nested struct fields should be flattened into the parent.
	Inline bool
	// Widget overrides the default widget selection (e.g. "input", "textarea",
	// "checkbox", "dropdown", "csv", "lines", or a custom name).
	Widget string
	// Options lists allowed choices for dropdown/enum widgets.
	Options []string
	// Help provides contextual documentation for the field.
	Help string
	// Placeholder provides input placeholder text.
	Placeholder string
	// ItemLabel names a child field used to summarize slice elements.
	ItemLabel string
	// Order controls relative field ordering (smaller values appear first).
	Order int
	// ReadOnly prevents editing the field in the UI.
	ReadOnly bool
	// Mask controls masking of sensitive field values.
	Mask MaskMode
	// Required marks the field as mandatory.
	Required bool
	// Expand hints that tree nodes for this field should start expanded.
	Expand bool
}

// Parse inspects field's yaml, doc/help, and tview tags and returns the merged
// Spec. tview tag directives override yaml tag defaults.
func Parse(field reflect.StructField) (Spec, error) {
	spec := Spec{
		FieldName: field.Name,
		Name:      field.Name,
	}

	yamlHidden := applyYAMLTag(field.Tag.Get("yaml"), &spec)
	if doc, ok := field.Tag.Lookup("doc"); ok {
		spec.Help = doc
	}
	if help, ok := field.Tag.Lookup("help"); ok {
		spec.Help = help
	}

	tviewTag, hasTView := field.Tag.Lookup("tview")
	if !hasTView {
		spec.Hidden = yamlHidden
		if spec.Label == "" {
			spec.Label = spec.Name
		}
		return spec, nil
	}

	override, err := applyTViewTag(tviewTag, &spec)
	if err != nil {
		return Spec{}, fmt.Errorf("field %s: %w", field.Name, err)
	}

	spec.Hidden = resolveHidden(yamlHidden, override)
	if spec.Label == "" {
		spec.Label = spec.Name
	}
	return spec, nil
}

func applyYAMLTag(raw string, spec *Spec) (hidden bool) {
	if raw == "" {
		return false
	}
	if raw == "-" {
		return true
	}
	parts := strings.Split(raw, ",")
	if parts[0] != "" {
		spec.YAMLName = parts[0]
		spec.Name = parts[0]
	}
	for _, opt := range parts[1:] {
		if opt == "inline" {
			spec.Inline = true
		}
	}
	return false
}

type hiddenOverride int

const (
	hiddenDefault hiddenOverride = iota
	hiddenForceHide
	hiddenForceShow
)

func resolveHidden(yamlHidden bool, override hiddenOverride) bool {
	switch override {
	case hiddenForceHide:
		return true
	case hiddenForceShow:
		return false
	default:
		return yamlHidden
	}
}

func applyTViewTag(raw string, spec *Spec) (hiddenOverride, error) {
	if raw == "" {
		return hiddenDefault, nil
	}
	if raw == "-" {
		return hiddenForceHide, nil
	}

	tokens, err := splitTokens(raw)
	if err != nil {
		return hiddenDefault, err
	}

	override := hiddenDefault
	for _, tok := range tokens {
		if tok == "" {
			continue
		}
		next, err := applyTViewToken(tok, spec)
		if err != nil {
			return hiddenDefault, err
		}
		if next != hiddenDefault {
			override = next
		}
	}
	return override, nil
}

func applyTViewToken(tok string, spec *Spec) (hiddenOverride, error) {
	key, val, hasVal := strings.Cut(tok, "=")
	if !hasVal {
		return applyTViewFlag(key, spec)
	}
	return applyTViewKV(key, val, spec)
}

func applyTViewFlag(flag string, spec *Spec) (hiddenOverride, error) {
	switch flag {
	case "-":
		return hiddenForceHide, nil
	case "show":
		return hiddenForceShow, nil
	case "inline":
		spec.Inline = true
	case "readonly":
		spec.ReadOnly = true
	case "always-hide":
		spec.Mask = MaskAlwaysHide
	case "hide-with-show":
		spec.Mask = MaskHideWithShow
	case "required":
		spec.Required = true
	case "expand":
		spec.Expand = true
	default:
		return hiddenDefault, fmt.Errorf("unknown tview tag flag %q", flag)
	}
	return hiddenDefault, nil
}

func applyTViewKV(key, val string, spec *Spec) (hiddenOverride, error) {
	switch key {
	case "label":
		spec.Label = val
		return hiddenForceShow, nil
	case "widget":
		spec.Widget = val
	case "options":
		if val == "" {
			spec.Options = nil
		} else {
			spec.Options = strings.Split(val, "|")
		}
	case "help", "desc":
		spec.Help = val
	case "placeholder":
		spec.Placeholder = val
	case "itemlabel":
		spec.ItemLabel = val
	case "order":
		n, err := strconv.Atoi(val)
		if err != nil {
			return hiddenDefault, fmt.Errorf("invalid order %q: %w", val, err)
		}
		spec.Order = n
	case "mask":
		return applyMaskKV(val, spec)
	case "inline", "readonly", "required", "expand":
		return applyBoolKV(key, val, spec)
	case "hidden":
		b, err := strconv.ParseBool(val)
		if err != nil {
			return hiddenDefault, fmt.Errorf("invalid hidden %q: %w", val, err)
		}
		if b {
			return hiddenForceHide, nil
		}
		return hiddenForceShow, nil
	default:
		return hiddenDefault, fmt.Errorf("unknown tview tag key %q", key)
	}
	return hiddenDefault, nil
}

func applyMaskKV(val string, spec *Spec) (hiddenOverride, error) {
	switch val {
	case "none", "":
		spec.Mask = MaskNone
	case "always-hide":
		spec.Mask = MaskAlwaysHide
	case "hide-with-show":
		spec.Mask = MaskHideWithShow
	default:
		return hiddenDefault, fmt.Errorf("invalid mask %q (want none, always-hide, or hide-with-show)", val)
	}
	return hiddenDefault, nil
}

func applyBoolKV(key, val string, spec *Spec) (hiddenOverride, error) {
	b, err := strconv.ParseBool(val)
	if err != nil {
		return hiddenDefault, fmt.Errorf("invalid %s %q: %w", key, val, err)
	}
	switch key {
	case "inline":
		spec.Inline = b
	case "readonly":
		spec.ReadOnly = b
	case "required":
		spec.Required = b
	case "expand":
		spec.Expand = b
	}
	return hiddenDefault, nil
}

// splitTokens splits a comma-separated tag value, preserving commas and spaces
// inside single-quoted segments ('...') and handling backslash escapes.
func splitTokens(s string) ([]string, error) {
	var tokens []string
	var buf strings.Builder
	inQuotes := false
	escaped := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			buf.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '\'' {
			inQuotes = !inQuotes
			continue
		}
		if ch == ',' && !inQuotes {
			tokens = append(tokens, strings.TrimSpace(buf.String()))
			buf.Reset()
			continue
		}
		buf.WriteByte(ch)
	}

	if escaped {
		return nil, fmt.Errorf("dangling escape in tag %q", s)
	}
	if inQuotes {
		return nil, fmt.Errorf("unclosed single quote in tag %q", s)
	}
	tokens = append(tokens, strings.TrimSpace(buf.String()))
	return tokens, nil
}
