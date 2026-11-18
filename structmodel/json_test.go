// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package structmodel_test

import (
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cloudeng.io/tui/tview/structmodel"
)

func TestMarshalAndApplyJSONFullSnapshot(t *testing.T) {
	src := sampleConfig{
		embeddedMeta: embeddedMeta{Owner: "platform", Env: "prod"},
		Host:         "api.internal",
		Enabled:      true,
		Timeout:      12 * time.Second,
		BindIP:       netip.MustParseAddr("10.1.2.3"),
		Tags:         []string{"edge", "v2"},
		Ports:        [2]int{80, 443},
		Routes: []route{
			{Name: "api", Port: 8080},
		},
		Endpoints: map[string]route{
			"primary": {Name: "main", Port: 9000},
		},
		Weights:  map[string]int{"primary": 100},
		Optional: &route{Name: "opt", Port: 7000},
	}

	data, err := structmodel.MarshalJSON(&src)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	text := string(data)
	for _, want := range []string{`"owner": "platform"`, `"env": "prod"`, `"timeout": "12s"`, `"bind_ip": "10.1.2.3"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected JSON to contain %s, got:\n%s", want, text)
		}
	}

	var dst sampleConfig
	if err := structmodel.ApplyJSON(&dst, data, structmodel.WithStrictJSON(true)); err != nil {
		t.Fatalf("ApplyJSON: %v", err)
	}
	if dst.Owner != "platform" || dst.Env != "prod" || dst.Host != "api.internal" ||
		!dst.Enabled || dst.Timeout != 12*time.Second || dst.BindIP != src.BindIP ||
		len(dst.Routes) != 1 || dst.Endpoints["primary"].Port != 9000 ||
		dst.Optional == nil || dst.Optional.Port != 7000 {
		t.Fatalf("unexpected dst after ApplyJSON: %+v", dst)
	}
}

func TestOverridesJSONPreservesUntouchedYAMLFields(t *testing.T) {
	// Simulated config parsed from a commented YAML file.
	yamlParsed := sampleConfig{
		embeddedMeta: embeddedMeta{Owner: "yaml-owner", Env: "staging"},
		Host:         "yaml.example.com",
		Enabled:      true,
		Timeout:      5 * time.Second,
		Endpoints: map[string]route{
			"prod":    {Name: "prod-api", Port: 443},
			"staging": {Name: "stage-api", Port: 8443},
		},
		Optional: &route{Name: "opt", Port: 7000},
	}

	// Copy and make targeted edits (change one field, one map entry field, delete one map key, clear Optional).
	edited := yamlParsed
	edited.Endpoints = map[string]route{
		"prod": {Name: "prod-api", Port: 9443},
	}
	edited.Owner = "tui-owner"
	edited.Optional = nil

	path := filepath.Join(t.TempDir(), "overrides.json")
	if err := structmodel.SaveOverridesJSONFile(path, &yamlParsed, &edited); err != nil {
		t.Fatalf("SaveOverridesJSONFile: %v", err)
	}

	// Layer a second override session on top of the existing file.
	second := edited
	second.Host = "override.example.com"
	if err := structmodel.SaveOverridesJSONFile(path, &edited, &second); err != nil {
		t.Fatalf("SaveOverridesJSONFile second: %v", err)
	}

	// Later, the user edits yaml.example.com's Timeout and Env in YAML directly.
	reloadedYAML := yamlParsed
	reloadedYAML.Timeout = 30 * time.Second
	reloadedYAML.Env = "prod"

	if err := structmodel.ApplyJSONFile(&reloadedYAML, path); err != nil {
		t.Fatalf("ApplyJSONFile: %v", err)
	}

	// Untouched YAML edits (Timeout=30s, Env="prod", prod.Name="prod-api") remain!
	if reloadedYAML.Timeout != 30*time.Second || reloadedYAML.Env != "prod" {
		t.Fatalf("expected untouched YAML fields to survive ApplyJSONFile, got %+v", reloadedYAML)
	}
	// Saved JSON overrides were applied!
	if reloadedYAML.Owner != "tui-owner" || reloadedYAML.Host != "override.example.com" ||
		reloadedYAML.Endpoints["prod"].Port != 9443 || reloadedYAML.Optional != nil {
		t.Fatalf("expected JSON overrides to apply, got %+v", reloadedYAML)
	}
	if _, exists := reloadedYAML.Endpoints["staging"]; exists {
		t.Fatal("expected deleted map key 'staging' to be removed")
	}
}

func TestDocumentOverridesJSONAndEnumsAndFlags(t *testing.T) {
	type enumAndFlagCfg struct {
		Wrapped enumWrapper[secretFormat] `yaml:"wrapped"`
		Direct  keychainKind              `yaml:"direct"`
		Flag    sampleFlagVal             `yaml:"flag"`
	}
	cfg := enumAndFlagCfg{
		Wrapped: enumWrapper[secretFormat]{Value: secretRaw},
		Direct:  keychainFile,
		Flag:    sampleFlagVal{raw: "v1"},
	}
	doc, err := structmodel.NewDocument(&cfg)
	if err != nil {
		t.Fatalf("NewDocument: %v", err)
	}
	byName := fieldMap(doc.Root().ScalarFields())
	mustSetString(t, byName["direct"], "icloud")
	mustSetString(t, byName["flag"], "v2")

	patch, err := doc.OverridesJSON()
	if err != nil {
		t.Fatalf("OverridesJSON: %v", err)
	}
	if strings.Contains(string(patch), "wrapped") {
		t.Fatalf("expected unchanged 'wrapped' to be omitted from overrides, got:\n%s", string(patch))
	}

	var restored enumAndFlagCfg
	restored.Wrapped = enumWrapper[secretFormat]{Value: secretHex}
	if err := structmodel.ApplyJSON(&restored, patch); err != nil {
		t.Fatalf("ApplyJSON: %v", err)
	}
	if restored.Wrapped.Value != secretHex || restored.Direct != keychainICloud || restored.Flag.raw != "v2" {
		t.Fatalf("unexpected restored enum/flag struct: %+v", restored)
	}
}
