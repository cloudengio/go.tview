// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"

	"cloudeng.io/cmdutil/cmdtypes"
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

func mustSetField(t *testing.T, node *structmodel.Node, name, val string) {
	t.Helper()
	if err := findField(t, node, name).SetString(val); err != nil {
		t.Fatalf("SetString(%s, %q): %v", name, val, err)
	}
}

func TestCmdTypesAndScalarFlags(t *testing.T) {
	cfg := DefaultConfig()
	ed, err := structedit.New(&cfg, structedit.WithTitle("cmdutil"))
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

	cmdt := findChild(t, root, "cmdtypes")
	mustSetField(t, cmdt, "byte_size", "64MiB")
	mustSetField(t, cmdt, "flex_time", "2026-12-01")
	mustSetField(t, cmdt, "permissions", "0640")

	sf := findChild(t, root, "scalar_flags")
	fmtField := findField(t, sf, "format")
	if got := strings.Join(fmtField.Tag().Options, ","); got != "json,text,yaml" {
		t.Fatalf("expected format options json,text,yaml, got %q", got)
	}
	mustSetField(t, sf, "format", "yaml")
	mustSetField(t, sf, "mode", "thorough")
	mustSetField(t, sf, "byte_size", "128MiB")
	mustSetField(t, sf, "time", "2026-12-25")
	mustSetField(t, sf, "permissions", "0700")

	if err := doc.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if cfg.CmdTypes.ByteSize != 64*cmdtypes.MiB || cfg.CmdTypes.Permissions != 0o640 {
		t.Fatalf("unexpected cmdtypes after commit: %+v", cfg.CmdTypes)
	}
	if cfg.ScalarFlags.Format.Value != FormatYAML || cfg.ScalarFlags.Mode != "thorough" {
		t.Fatalf("unexpected scalar flags after commit: %+v", cfg.ScalarFlags)
	}
	if cfg.ScalarFlags.ByteSize.String() != "128MiB" || cfg.ScalarFlags.Permissions.Octal() != "0700" {
		t.Fatalf("unexpected flag.Value fields after commit: %s %s",
			cfg.ScalarFlags.ByteSize.String(), cfg.ScalarFlags.Permissions.Octal())
	}
}

func TestBoundsIntervalsAndMultiFlags(t *testing.T) {
	cfg := DefaultConfig()
	doc, err := structmodel.NewDocument(&cfg)
	if err != nil {
		t.Fatalf("NewDocument: %v", err)
	}
	root := doc.Root()

	bounds := findChild(t, root, "bounds")
	mustSetField(t, bounds, "int_bound", "42")
	mustSetField(t, bounds, "byte_size_bound", "2GiB")
	mustSetField(t, bounds, "duration_bound", "45m")
	mustSetField(t, bounds, "time_bound", "2026-06-01")
	mustSetField(t, bounds, "string_bound", "z")

	intervals := findChild(t, root, "intervals")
	mustSetField(t, intervals, "int_interval", "10..20")
	mustSetField(t, intervals, "byte_size_interval", "2KiB..8MiB")
	mustSetField(t, intervals, "duration_interval", "5m..1h")
	mustSetField(t, intervals, "time_interval", "2026-02-01..2026-02-28")
	mustSetField(t, intervals, "string_interval", "c..x")
	mustSetField(t, intervals, "int_intervals", "1..2,5..8")
	mustSetField(t, intervals, "byte_size_intervals", "1KiB..2KiB,4KiB..8KiB")
	mustSetField(t, intervals, "duration_intervals", "1s..2s,5s..10s")
	mustSetField(t, intervals, "time_intervals", "2026-01-01..2026-01-05")
	mustSetField(t, intervals, "string_intervals", "a..c,x..z")

	multi := findChild(t, root, "multi_flags")
	repVals := findChild(t, findChild(t, multi, "repeating"), "Values")
	if _, err := repVals.AppendItem(); err != nil {
		t.Fatalf("AppendItem repeating: %v", err)
	}
	if err := repVals.SetItemScalar(2, "gamma"); err != nil {
		t.Fatalf("SetItemScalar repeating: %v", err)
	}

	if err := doc.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if cfg.Bounds.Int.Lower() != 42 || cfg.Intervals.Int.String() != "10..20" ||
		cfg.Intervals.IntList.String() != "1..2,5..8" || len(cfg.MultiFlags.Repeating.Values) != 3 {
		t.Fatalf("unexpected bounds/intervals/multi after commit: %+v %+v %+v",
			cfg.Bounds, cfg.Intervals, cfg.MultiFlags)
	}
}

func TestOneOfValidationError(t *testing.T) {
	cfg := DefaultConfig()
	doc, err := structmodel.NewDocument(&cfg)
	if err != nil {
		t.Fatalf("NewDocument: %v", err)
	}
	sf := findChild(t, doc.Root(), "scalar_flags")
	mustSetField(t, sf, "mode", "invalid-mode")
	if err := doc.Commit(); err == nil {
		t.Fatal("expected Validate() to reject invalid OneOf mode")
	}
}
