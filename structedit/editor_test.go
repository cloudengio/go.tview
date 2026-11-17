// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package structedit_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"cloudeng.io/tui/tview/structedit"
	"cloudeng.io/tui/tview/structmodel"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type testRoute struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
}

type testNode struct {
	Label    string      `yaml:"label"`
	Next     *testNode   `yaml:"next"`
	Children []*testNode `yaml:"children" tview:"itemlabel=Label"`
}

type testConfig struct {
	Host     string               `yaml:"host" tview:"help='Server hostname'"`
	Level    string               `yaml:"level" tview:"widget=dropdown,options=debug|info|warn"`
	Enabled  bool                 `yaml:"enabled"`
	Timeout  time.Duration        `yaml:"timeout"`
	Secret   string               `yaml:"-" tview:"show,always-hide,label='Token'"`
	Tags     []string             `yaml:"tags" tview:"widget=csv"`
	Routes   []testRoute          `yaml:"routes" tview:"itemlabel=Name"`
	Backends map[string]testRoute `yaml:"backends" tview:"itemlabel=Name"`
	Optional *testRoute           `yaml:"optional"`
	Tree     *testNode            `yaml:"tree"`
}

func renderScreenText(screen tcell.SimulationScreen) string {
	w, h := screen.Size()
	var sb strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, _, _, _ := screen.GetContent(x, y)
			if r == 0 {
				sb.WriteByte(' ')
			} else {
				sb.WriteRune(r)
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func newTestScreen(t *testing.T, w, h int) tcell.SimulationScreen {
	t.Helper()
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatalf("SimulationScreen.Init: %v", err)
	}
	scr.SetSize(w, h)
	t.Cleanup(scr.Fini)
	return scr
}

func makeFocusSetter(focused *tview.Primitive) func(tview.Primitive) {
	var setFocus func(tview.Primitive)
	setFocus = func(p tview.Primitive) {
		if *focused != nil && *focused != p {
			(*focused).Blur()
		}
		*focused = p
		if p != nil {
			p.Focus(setFocus)
		}
	}
	return setFocus
}

func sendKey(ed *structedit.Editor, setFocus func(tview.Primitive), key tcell.Key, r rune, mod tcell.ModMask) {
	ev := tcell.NewEventKey(key, r, mod)
	if h := ed.InputHandler(); h != nil {
		h(ev, setFocus)
	}
}

func TestSplitEditorRenderAndEditSave(t *testing.T) {
	n1 := &testNode{Label: "n1"}
	n1.Next = n1 // cycle!

	cfg := testConfig{
		Host:    "localhost",
		Level:   "info",
		Enabled: true,
		Timeout: 5 * time.Second,
		Secret:  "topsecret",
		Tags:    []string{"prod", "us-east"},
		Routes:  []testRoute{{Name: "api", Port: 8080}},
		Backends: map[string]testRoute{
			"primary": {Name: "main", Port: 9000},
		},
		Tree: n1,
	}

	var saved bool
	ed, err := structedit.New(
		&cfg,
		structedit.WithTitle("TestConfig"),
		structedit.WithOnSave(func(_ any) error {
			saved = true
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("structedit.New: %v", err)
	}

	scr := newTestScreen(t, 120, 35)
	var focused tview.Primitive
	setFocus := makeFocusSetter(&focused)
	setFocus(ed)
	ed.SetRect(0, 0, 120, 35)
	ed.Draw(scr)
	scr.Show()

	out := renderScreenText(scr)
	for _, want := range []string{"TestConfig", "Structure", "host", "routes [1]", "backends {1}", "optional [nil]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected screen to contain %q, got:\n%s", want, out)
		}
	}

	// Press Enter on root node in tree -> moves focus to right-hand form (Host field).
	sendKey(ed, setFocus, tcell.KeyEnter, 0, tcell.ModNone)
	ed.Draw(scr)

	// Append "-new" to Host input field.
	for _, ch := range "-new" {
		sendKey(ed, setFocus, tcell.KeyRune, ch, tcell.ModNone)
	}
	if !ed.Document().Dirty() {
		t.Fatal("expected document to become dirty after typing")
	}

	// Press Ctrl+S to Save.
	sendKey(ed, setFocus, tcell.KeyCtrlS, 0, tcell.ModNone)
	if !saved {
		t.Fatal("expected WithOnSave callback to fire on Ctrl+S")
	}
	if cfg.Host != "localhost-new" {
		t.Fatalf("expected committed Host 'localhost-new', got %q", cfg.Host)
	}
}

func TestSplitEditorTreeNavigationSlicesMapsAndCustomWidget(t *testing.T) {
	cfg := testConfig{
		Host:   "example.com",
		Routes: []testRoute{{Name: "web", Port: 80}},
		Backends: map[string]testRoute{
			"alpha": {Name: "a", Port: 7000},
		},
	}

	customCalled := false
	ed, err := structedit.New(
		&cfg,
		structedit.WithTypeWidget(reflect.TypeFor[time.Duration](), func(fb *structmodel.FieldBinding, notify func()) tview.FormItem {
			customCalled = true
			in := tview.NewInputField().SetLabel(fb.Tag().Label).SetText(fb.String())
			in.SetChangedFunc(func(text string) {
				_ = fb.SetString(text)
				notify()
			})
			return in
		}),
	)
	if err != nil {
		t.Fatalf("structedit.New: %v", err)
	}
	if !customCalled {
		t.Fatal("expected custom TypeWidget factory to be invoked")
	}

	scr := newTestScreen(t, 120, 35)
	var focused tview.Primitive
	setFocus := makeFocusSetter(&focused)
	setFocus(ed)
	ed.SetRect(0, 0, 120, 35)
	ed.Draw(scr)

	// Move down in tree to "routes [1]" (child index 0) and press Enter to focus slice table.
	sendKey(ed, setFocus, tcell.KeyDown, 0, tcell.ModNone)
	sendKey(ed, setFocus, tcell.KeyEnter, 0, tcell.ModNone)
	ed.Draw(scr)
	out := renderScreenText(scr)
	if !strings.Contains(out, "web") {
		t.Fatalf("expected slice table to show 'web', got:\n%s", out)
	}

	// Press 'a' on slice table to append a new route item.
	sendKey(ed, setFocus, tcell.KeyRune, 'a', tcell.ModNone)
	if err := ed.Save(); err != nil {
		t.Fatalf("Save after append: %v", err)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("expected 2 routes after append+save, got %d", len(cfg.Routes))
	}

	// Revert test.
	ed.Revert()
	if ed.Document().Dirty() {
		t.Fatal("expected clean document after Revert")
	}
}

func TestSplitEditorMapNilPtrAndCycle(t *testing.T) {
	n1 := &testNode{Label: "root"}
	n1.Next = n1

	cfg := testConfig{
		Host: "example.com",
		Backends: map[string]testRoute{
			"alpha": {Name: "a", Port: 7000},
		},
		Tree: n1,
	}

	ed, err := structedit.New(&cfg)
	if err != nil {
		t.Fatalf("structedit.New: %v", err)
	}

	scr := newTestScreen(t, 120, 35)
	var focused tview.Primitive
	setFocus := makeFocusSetter(&focused)
	setFocus(ed)
	ed.SetRect(0, 0, 120, 35)
	ed.Draw(scr)

	// Move down twice to "backends {1}", press Enter to open map view.
	sendKey(ed, setFocus, tcell.KeyDown, 0, tcell.ModNone)
	sendKey(ed, setFocus, tcell.KeyDown, 0, tcell.ModNone)
	sendKey(ed, setFocus, tcell.KeyEnter, 0, tcell.ModNone)
	ed.Draw(scr)
	if out := renderScreenText(scr); !strings.Contains(out, "alpha") {
		t.Fatalf("expected map table to show 'alpha', got:\n%s", out)
	}

	// Press 'a' to open Add Map Entry modal, type "beta", press Enter on OK.
	sendKey(ed, setFocus, tcell.KeyRune, 'a', tcell.ModNone)
	ed.Draw(scr)
	for _, ch := range "beta" {
		sendKey(ed, setFocus, tcell.KeyRune, ch, tcell.ModNone)
	}
	sendKey(ed, setFocus, tcell.KeyTab, 0, tcell.ModNone)
	sendKey(ed, setFocus, tcell.KeyEnter, 0, tcell.ModNone)
	if err := ed.Save(); err != nil {
		t.Fatalf("Save after adding map key: %v", err)
	}
	if _, ok := cfg.Backends["beta"]; !ok {
		t.Fatalf("expected map key 'beta' to exist after Save, got %+v", cfg.Backends)
	}

	// Return to tree, move past alpha & beta to "optional [nil]" and press Enter + Enter on Initialize.
	sendKey(ed, setFocus, tcell.KeyEscape, 0, tcell.ModNone)
	sendKey(ed, setFocus, tcell.KeyDown, 0, tcell.ModNone)
	sendKey(ed, setFocus, tcell.KeyDown, 0, tcell.ModNone)
	sendKey(ed, setFocus, tcell.KeyDown, 0, tcell.ModNone)
	sendKey(ed, setFocus, tcell.KeyEnter, 0, tcell.ModNone)
	ed.Draw(scr)
	if out := renderScreenText(scr); !strings.Contains(out, "Initialize") {
		t.Fatalf("expected Initialize button for nil pointer, got:\n%s", out)
	}
	sendKey(ed, setFocus, tcell.KeyEnter, 0, tcell.ModNone)
	if err := ed.Save(); err != nil {
		t.Fatalf("Save after InitPtr: %v", err)
	}
	if cfg.Optional == nil {
		t.Fatal("expected Optional pointer to be initialized after Save")
	}

	// Return to tree, move to "tree", expand Right, move down to "next [cycle]".
	sendKey(ed, setFocus, tcell.KeyEscape, 0, tcell.ModNone)
	sendKey(ed, setFocus, tcell.KeyDown, 0, tcell.ModNone)
	sendKey(ed, setFocus, tcell.KeyRight, 0, tcell.ModNone)
	sendKey(ed, setFocus, tcell.KeyDown, 0, tcell.ModNone)
	ed.Draw(scr)
	if out := renderScreenText(scr); !strings.Contains(out, "Recursive pointer cycle detected") {
		t.Fatalf("expected cycle warning for recursive pointer, got:\n%s", out)
	}
}

type uiEnum int

const (
	uiEnumAlpha uiEnum = iota + 1
	uiEnumBeta
)

func (uiEnum) EnumValues() map[string]uiEnum {
	return map[string]uiEnum{"alpha": uiEnumAlpha, "beta": uiEnumBeta}
}

func TestSplitEditorEnumDropDown(t *testing.T) {
	type enumOnly struct {
		Mode uiEnum `yaml:"mode"`
	}
	cfg := enumOnly{Mode: uiEnumAlpha}
	ed, err := structedit.New(&cfg)
	if err != nil {
		t.Fatalf("structedit.New: %v", err)
	}
	scr := newTestScreen(t, 80, 20)
	var focused tview.Primitive
	setFocus := makeFocusSetter(&focused)
	setFocus(ed)
	ed.SetRect(0, 0, 80, 20)
	ed.Draw(scr)
	if out := renderScreenText(scr); !strings.Contains(out, "alpha") {
		t.Fatalf("expected enum dropdown to display 'alpha', got:\n%s", out)
	}
	if ed.Document().Dirty() {
		t.Fatal("expected clean document before user interaction")
	}
}

func TestSplitEditorMaskModes(t *testing.T) {
	type maskConfig struct {
		KeyID       string `yaml:"key_id"`
		AlwaysHide  string `yaml:"always_hide" tview:"always-hide"`
		HideAndShow string `yaml:"hide_and_show" tview:"hide-with-show"`
	}
	cfg := maskConfig{
		KeyID:       "key-1234",
		AlwaysHide:  "very-long-secret-value-not-leaking-length",
		HideAndShow: "reveal-me",
	}
	ed, err := structedit.New(&cfg)
	if err != nil {
		t.Fatalf("structedit.New: %v", err)
	}

	scr := newTestScreen(t, 100, 25)
	var focused tview.Primitive
	setFocus := makeFocusSetter(&focused)
	setFocus(ed)
	ed.SetRect(0, 0, 100, 25)
	ed.Draw(scr)

	out := renderScreenText(scr)
	if !strings.Contains(out, "key-1234") {
		t.Fatalf("expected plain KeyID value 'key-1234' to be visible, got:\n%s", out)
	}
	if strings.Contains(out, "very-long-secret") || strings.Contains(out, "reveal-me") {
		t.Fatalf("expected masked fields not to expose secrets initially, got:\n%s", out)
	}
	// always-hide shows fixed 8 asterisks (not 41 asterisks matching secret length).
	if !strings.Contains(out, "********") || strings.Contains(out, strings.Repeat("*", 12)) {
		t.Fatalf("expected fixed 8-asterisk mask for always-hide, got:\n%s", out)
	}

	// Focus form, tab to always-hide, type replacement "abc", tab away -> Blur resets display to 8 '*'.
	sendKey(ed, setFocus, tcell.KeyEnter, 0, tcell.ModNone)
	sendKey(ed, setFocus, tcell.KeyTab, 0, tcell.ModNone)
	for _, ch := range "abc" {
		sendKey(ed, setFocus, tcell.KeyRune, ch, tcell.ModNone)
	}
	sendKey(ed, setFocus, tcell.KeyTab, 0, tcell.ModNone)
	ed.Draw(scr)
	if err := ed.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if cfg.AlwaysHide != "abc" {
		t.Fatalf("expected AlwaysHide 'abc', got %q", cfg.AlwaysHide)
	}

	// Currently focused on hide_and_show input; press Ctrl+R to reveal "reveal-me".
	sendKey(ed, setFocus, tcell.KeyCtrlR, 0, tcell.ModNone)
	ed.Draw(scr)
	if out = renderScreenText(scr); !strings.Contains(out, "reveal-me") {
		t.Fatalf("expected Ctrl+R to reveal hide-with-show value 'reveal-me', got:\n%s", out)
	}

	// Tab to "Show hide_and_show" checkbox and toggle it back off.
	sendKey(ed, setFocus, tcell.KeyTab, 0, tcell.ModNone)
	sendKey(ed, setFocus, tcell.KeyRune, ' ', tcell.ModNone)
	ed.Draw(scr)
	if out = renderScreenText(scr); strings.Contains(out, "reveal-me") {
		t.Fatalf("expected checkbox toggle to re-hide value, got:\n%s", out)
	}
}
