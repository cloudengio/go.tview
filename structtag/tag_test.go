// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package structtag_test

import (
	"reflect"
	"testing"

	"cloudeng.io/tui/tview/structtag"
)

type sampleTags struct {
	Plain       string
	KeyID       string   `yaml:"key_id"`
	YAMLNamed   string   `yaml:"yaml_named,omitempty"`
	YAMLHidden  string   `yaml:"-"`
	LiteralDash string   `yaml:"-,"`
	TViewShow   string   `yaml:"-" tview:"show,label='Shown Field'"`
	TViewHide   string   `yaml:"exposed" tview:"-"`
	Inlined     struct{} `yaml:",inline"`
	NotInlined  struct{} `yaml:",inline" tview:"inline=false,order=-1"`
	AlwaysHide  string   `yaml:"api_key" tview:"always-hide,required,placeholder='sk-...',help='Secret, with comma'"`
	HideShow    string   `yaml:"token" tview:"hide-with-show"`
	MaskKV      string   `yaml:"mask_kv" tview:"mask=hide-with-show"`
	Dropdown    string   `yaml:"level" doc:"Doc fallback" tview:"widget=dropdown,options=debug|info|warn|error,expand"`
	SliceField  []int    `yaml:"ports" tview:"itemlabel=Name,readonly,order=5"`
}

func parseFieldSpec(t *testing.T, rt reflect.Type, name string) structtag.Spec {
	t.Helper()
	sf, ok := rt.FieldByName(name)
	if !ok {
		t.Fatalf("missing field %s", name)
	}
	got, err := structtag.Parse(sf)
	if err != nil {
		t.Fatalf("Parse(%s): %v", name, err)
	}
	return got
}

func TestParseYAMLAndVisibility(t *testing.T) {
	rt := reflect.TypeFor[sampleTags]()

	if got := parseFieldSpec(t, rt, "Plain"); got.Name != "Plain" || got.Label != "Plain" || got.Hidden {
		t.Fatalf("Plain: unexpected spec: %+v", got)
	}
	if got := parseFieldSpec(t, rt, "YAMLNamed"); got.YAMLName != "yaml_named" || got.Name != "yaml_named" || got.Label != "yaml_named" {
		t.Fatalf("YAMLNamed: unexpected spec: %+v", got)
	}
	if got := parseFieldSpec(t, rt, "YAMLHidden"); !got.Hidden {
		t.Fatalf("YAMLHidden: expected hidden=true, got %+v", got)
	}
	if got := parseFieldSpec(t, rt, "LiteralDash"); got.Hidden || got.Name != "-" {
		t.Fatalf("LiteralDash: expected literal '-', got %+v", got)
	}
	if got := parseFieldSpec(t, rt, "TViewShow"); got.Hidden || got.Label != "Shown Field" {
		t.Fatalf("TViewShow: unexpected spec: %+v", got)
	}
	if got := parseFieldSpec(t, rt, "TViewHide"); !got.Hidden || got.Name != "exposed" {
		t.Fatalf("TViewHide: unexpected spec: %+v", got)
	}
}

func TestParseInlineAndOrder(t *testing.T) {
	rt := reflect.TypeFor[sampleTags]()

	if got := parseFieldSpec(t, rt, "Inlined"); !got.Inline {
		t.Fatalf("Inlined: expected inline=true, got %+v", got)
	}
	if got := parseFieldSpec(t, rt, "NotInlined"); got.Inline || got.Order != -1 {
		t.Fatalf("NotInlined: expected inline=false order=-1, got %+v", got)
	}
	gotSlice := parseFieldSpec(t, rt, "SliceField")
	if gotSlice.ItemLabel != "Name" || !gotSlice.ReadOnly || gotSlice.Order != 5 {
		t.Fatalf("SliceField: unexpected spec: %+v", gotSlice)
	}
}

func TestParseMaskAndWidgets(t *testing.T) {
	rt := reflect.TypeFor[sampleTags]()

	if got := parseFieldSpec(t, rt, "KeyID"); got.Mask != structtag.MaskNone {
		t.Fatalf("KeyID: expected MaskNone, got %+v", got)
	}

	gotAlways := parseFieldSpec(t, rt, "AlwaysHide")
	if gotAlways.Mask != structtag.MaskAlwaysHide || !gotAlways.Required {
		t.Fatalf("AlwaysHide flags: unexpected spec: %+v", gotAlways)
	}
	if gotAlways.Placeholder != "sk-..." || gotAlways.Help != "Secret, with comma" {
		t.Fatalf("AlwaysHide text: unexpected spec: %+v", gotAlways)
	}
	if got := parseFieldSpec(t, rt, "HideShow"); got.Mask != structtag.MaskHideWithShow {
		t.Fatalf("HideShow: unexpected spec: %+v", got)
	}
	if got := parseFieldSpec(t, rt, "MaskKV"); got.Mask != structtag.MaskHideWithShow {
		t.Fatalf("MaskKV: unexpected spec: %+v", got)
	}
	gotDrop := parseFieldSpec(t, rt, "Dropdown")
	if gotDrop.Help != "Doc fallback" || gotDrop.Widget != "dropdown" || !gotDrop.Expand {
		t.Fatalf("Dropdown flags: unexpected spec: %+v", gotDrop)
	}
	if len(gotDrop.Options) != 4 || gotDrop.Options[1] != "info" {
		t.Fatalf("Dropdown options: unexpected spec: %+v", gotDrop)
	}
}

func TestParseErrors(t *testing.T) {
	type badTags struct {
		UnknownFlag string `tview:"bogus"`
		UnknownKey  string `tview:"foo=bar"`
		BadOrder    string `tview:"order=nope"`
		BadBool     string `tview:"inline=maybe"`
		BadMask     string `tview:"mask=invalid"`
		Unclosed    string `tview:"label='abc"`
		Dangling    string `tview:"label=abc\\"`
	}
	rt := reflect.TypeFor[badTags]()
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		if _, err := structtag.Parse(sf); err == nil {
			t.Errorf("expected error for field %s", sf.Name)
		}
	}
}
