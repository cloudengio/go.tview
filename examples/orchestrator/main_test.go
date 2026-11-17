// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"

	"cloudeng.io/tui/tview/structedit"
	"cloudeng.io/tui/tview/structmodel"
	"github.com/gdamore/tcell/v2"
)

func findChild(t *testing.T, parent *structmodel.Node, name string) *structmodel.Node {
	t.Helper()
	for _, ch := range parent.Children() {
		if ch.Tag().Name == name || ch.Label() == name {
			return ch
		}
	}
	t.Fatalf("missing child %q under %s", name, parent.Path())
	return nil
}

func findField(t *testing.T, node *structmodel.Node, name string) *structmodel.FieldBinding {
	t.Helper()
	for _, f := range node.ScalarFields() {
		if f.Tag().Name == name {
			return f
		}
	}
	t.Fatalf("missing field %q on %s", name, node.Path())
	return nil
}

func TestOrchestratorConfigSchemaAndDeepEdit(t *testing.T) {
	cfg := DefaultConfig()
	ed, err := structedit.New(&cfg, structedit.WithTitle("Orchestrator"))
	if err != nil {
		t.Fatalf("structedit.New: %v", err)
	}

	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatalf("SimulationScreen.Init: %v", err)
	}
	defer scr.Fini()
	scr.SetSize(120, 40)
	ed.SetRect(0, 0, 120, 40)
	ed.Draw(scr)

	doc := ed.Document()
	root := doc.Root()

	// Check enum extraction on logging.format (flags.Enum-style) and icloud_keychain.keychain_type.
	logging := findChild(t, root, "logging")
	fmtField := findField(t, logging, "format")
	if got := strings.Join(fmtField.Tag().Options, ","); got != "json,text" {
		t.Fatalf("expected logging.format options json,text, got %q", got)
	}
	if err := fmtField.SetString("json"); err != nil {
		t.Fatalf("SetString logging.format: %v", err)
	}

	keychain := findChild(t, root, "icloud_keychain")
	ktField := findField(t, keychain, "keychain_type")
	if len(ktField.Tag().Options) != 4 {
		t.Fatalf("expected 4 keychain_type options, got %v", ktField.Tag().Options)
	}

	// Mutate deeply nested field inside map[string]PoolConfig -> *TartConfig -> *VerificationPolicy -> []Signer.
	pools := findChild(t, root, "vm_pools")
	macos := findChild(t, pools, "macos")
	staging := findField(t, macos, "staging_behaviour")
	if err := staging.SetString("Suspended"); err != nil {
		t.Fatalf("SetString staging_behaviour: %v", err)
	}
	tart := findChild(t, macos, "tart_config")
	verify := findChild(t, tart, "verify")
	signers := findChild(t, verify, "signers")
	firstSigner := signers.Children()[0]
	if err := findField(t, firstSigner, "email").SetString("release@cloudeng.io"); err != nil {
		t.Fatalf("SetString signer email: %v", err)
	}

	if err := doc.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if cfg.Logging.Format.Value != "json" {
		t.Fatalf("expected Logging.Format=json, got %q", cfg.Logging.Format.Value)
	}
	if cfg.VMPools["macos"].StagingBehaviour != StagingBehaviourSuspended {
		t.Fatalf("expected StagingBehaviourSuspended, got %v", cfg.VMPools["macos"].StagingBehaviour)
	}
	if got := cfg.VMPools["macos"].Tart.Verify.Signers[0].Email; got != "release@cloudeng.io" {
		t.Fatalf("expected updated signer email, got %q", got)
	}
}

func TestOrchestratorConfigValidationFailure(t *testing.T) {
	cfg := DefaultConfig()
	doc, err := structmodel.NewDocument(&cfg)
	if err != nil {
		t.Fatalf("NewDocument: %v", err)
	}
	repos := findChild(t, doc.Root(), "repositories")
	runners := findChild(t, repos.Children()[0], "runners")
	if err := findField(t, runners.Children()[0], "vm_pool").SetString("nonexistent"); err != nil {
		t.Fatalf("SetString vm_pool: %v", err)
	}
	if err := doc.Commit(); err == nil {
		t.Fatal("expected Config.Validate to reject unknown vm_pool")
	}
}
