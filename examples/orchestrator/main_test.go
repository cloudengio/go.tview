// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"

	"cloudeng.io/tui/tview/structedit"
	"cloudeng.io/tui/tview/structmodel"
	basepool "cloudeng.io/vms/vmspool"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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

func mustSetField(t *testing.T, node *structmodel.Node, name, val string) {
	t.Helper()
	if err := findField(t, node, name).SetString(val); err != nil {
		t.Fatalf("SetString(%s, %q): %v", name, val, err)
	}
}

func TestOrchestratorConfigEnumsAndLogging(t *testing.T) {
	cfg := DefaultConfig()
	ed, err := structedit.New(&cfg, editorOptions(nil)...)
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

	logging := findChild(t, root, "logging")
	mustSetField(t, logging, "format", "json")
	mustSetField(t, logging, "max_size", "25MB")

	keychain := findChild(t, root, "icloud_keychain")
	ktField := findField(t, keychain, "keychain_type")
	if len(ktField.Tag().Options) != 4 {
		t.Fatalf("expected 4 keychain_type options from flags.Enum, got %v", ktField.Tag().Options)
	}
	mustSetField(t, keychain, "keychain_type", "icloud")

	if err := doc.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if cfg.Logging.Format != "json" {
		t.Fatalf("expected Logging.Format=json, got %q", cfg.Logging.Format)
	}
	if cfg.ICloudKeychain.Type.Value != KeychainICloud {
		t.Fatalf("expected KeychainICloud, got %v", cfg.ICloudKeychain.Type.Value)
	}
}

func TestOrchestratorConfigDeepEdit(t *testing.T) {
	cfg := DefaultConfig()
	doc, err := structmodel.NewDocument(&cfg)
	if err != nil {
		t.Fatalf("NewDocument: %v", err)
	}

	pools := findChild(t, doc.Root(), "vm_pools")
	macos := findChild(t, pools, "macos")
	mustSetField(t, macos, "staging_behaviour", "Suspended")

	tart := findChild(t, macos, "tart_config")
	verify := findChild(t, tart, "verify")
	firstSigner := findChild(t, verify, "signers").Children()[0]
	mustSetField(t, firstSigner, "key", "")
	mustSetField(t, firstSigner, "issuer", "https://token.actions.githubusercontent.com")
	mustSetField(t, firstSigner, "subject", "release@cloudeng.io")

	if err := doc.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if cfg.VMPools["macos"].StagingBehaviour != basepool.StagingBehaviourSuspended {
		t.Fatalf("expected StagingBehaviourSuspended, got %v", cfg.VMPools["macos"].StagingBehaviour)
	}
	if got := cfg.VMPools["macos"].Tart.Verify.Signers[0].Subject; got != "release@cloudeng.io" {
		t.Fatalf("expected updated signer subject, got %q", got)
	}
}

func TestOrchestratorValidationFailure(t *testing.T) {
	cfg := DefaultConfig()
	doc, err := structmodel.NewDocument(&cfg)
	if err != nil {
		t.Fatalf("NewDocument: %v", err)
	}
	repos := findChild(t, doc.Root(), "repositories")
	runners := findChild(t, repos.Children()[0], "runners")
	mustSetField(t, runners.Children()[0], "vm_pool", "nonexistent")
	if err := doc.Commit(); err == nil {
		t.Fatal("expected Config.Validate to reject unknown vm_pool")
	}
}

func TestOrchestratorThreePaneRepositories(t *testing.T) {
	cfg := DefaultConfig()
	ed, err := structedit.New(&cfg, editorOptions(nil)...)
	if err != nil {
		t.Fatalf("structedit.New: %v", err)
	}
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatalf("SimulationScreen.Init: %v", err)
	}
	defer scr.Fini()
	scr.SetSize(140, 40)

	var focused tview.Primitive
	var setFocus func(tview.Primitive)
	setFocus = func(p tview.Primitive) {
		if focused != nil && focused != p {
			focused.Blur()
		}
		focused = p
		if p != nil {
			p.Focus(setFocus)
		}
	}
	setFocus(ed)
	ed.SetRect(0, 0, 140, 40)
	ed.Draw(scr)

	// Move down to repositories [2] (logging, global, repositories).
	for range 3 {
		if h := ed.InputHandler(); h != nil {
			h(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), setFocus)
		}
	}
	ed.Draw(scr)
	scr.Show()
	cells, w, h := scr.GetContents()
	var sb strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			runes := cells[y*w+x].Runes
			if len(runes) == 0 || runes[0] == 0 {
				sb.WriteByte(' ')
			} else {
				sb.WriteRune(runes[0])
			}
		}
		sb.WriteByte('\n')
	}
	out := sb.String()
	for _, want := range []string{"Structure", "Items [a:Add d:Del u/j:Move]", "Item [0]", "go.pkgs", "Add", "Del"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected 3-pane orchestrator view to contain %q, got:\n%s", want, out)
		}
	}
}
