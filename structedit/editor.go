// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package structedit

import (
	"fmt"
	"strings"

	"cloudeng.io/tui/tview/structmodel"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	mainPageName  = "main"
	modalPageName = "modal"
)

// Editor is a split-view tview.Primitive for editing an arbitrary Go struct
// via yaml and tview struct tags.
type Editor struct {
	*tview.Pages

	cfg      *editorConfig
	target   any
	doc      *structmodel.Document
	rootFlex *tview.Flex
	header   *tview.TextView
	tree     *tview.TreeView
	detail   *tview.Pages
	helpView *tview.TextView
	footer   *tview.TextView

	currentNode    *structmodel.Node
	activeForm     *tview.Form
	activeBindings []*structmodel.FieldBinding
	activeList     tview.Primitive
	activeItemPane tview.Primitive
	currentHelp    string
	currentOptions []string
	currentMeta    string
	currentErr     error
	focusDelegate  func(tview.Primitive)
}

// New constructs a split-view Editor bound to target, which must be a non-nil
// pointer to a struct.
func New(target any, opts ...Option) (*Editor, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	docOpts := []structmodel.Option{
		structmodel.WithCodecs(cfg.codecs),
		structmodel.WithRootLabel(cfg.title),
	}
	for t, enumOpts := range cfg.enumOptions {
		docOpts = append(docOpts, structmodel.WithEnumOptions(t, enumOpts))
	}
	if cfg.validator != nil {
		docOpts = append(docOpts, structmodel.WithValidator(cfg.validator))
	}
	doc, err := structmodel.NewDocument(target, docOpts...)
	if err != nil {
		return nil, err
	}

	ed := &Editor{
		Pages:  tview.NewPages(),
		cfg:    cfg,
		target: target,
		doc:    doc,
	}
	ed.buildLayout()
	ed.initTree()
	ed.selectModelNode(doc.Root(), false)
	ed.updateHeader()
	ed.updateFooter()
	return ed, nil
}

// Document returns the underlying structmodel.Document.
func (e *Editor) Document() *structmodel.Document {
	return e.doc
}

// Save validates and commits all staged changes back into the target struct,
// then invokes the WithOnSave callback if configured.
func (e *Editor) Save() error {
	if e.cfg.readOnly {
		return nil
	}
	if err := e.doc.Commit(); err != nil {
		e.currentErr = err
		e.updateFooter()
		e.showMessageModal("Validation Error", err.Error(), []string{"OK"}, nil)
		return err
	}
	e.currentErr = nil
	e.refreshTreeLabels()
	e.updateHeader()
	e.updateFooter()
	if e.cfg.onSave != nil {
		if err := e.cfg.onSave(e.target); err != nil {
			e.currentErr = err
			e.updateFooter()
			e.showMessageModal("Save Failed", err.Error(), []string{"OK"}, nil)
			return err
		}
	}
	return nil
}

// Cancel requests cancellation. If unsaved edits exist, a confirmation modal
// is shown unless read-only.
func (e *Editor) Cancel() {
	if !e.doc.Dirty() || e.cfg.readOnly {
		if e.cfg.onCancel != nil {
			e.cfg.onCancel()
		}
		return
	}
	e.showMessageModal(
		"Discard Changes?",
		"You have unsaved changes. Discard them?",
		[]string{"Discard", "Keep Editing"},
		func(label string) {
			if label == "Discard" {
				e.doc.Revert()
				if e.cfg.onCancel != nil {
					e.cfg.onCancel()
				}
			}
		},
	)
}

// Revert discards staged edits and refreshes the entire editor UI.
func (e *Editor) Revert() {
	e.doc.Revert()
	e.currentErr = nil
	e.initTree()
	e.selectModelNode(e.doc.Root(), false)
	e.updateHeader()
	e.updateFooter()
}

// Draw synchronizes the description panel with the currently focused form field
// before drawing the editor.
func (e *Editor) Draw(screen tcell.Screen) {
	e.syncActiveFieldHelp()
	e.Pages.Draw(screen)
}

// Focus delegates initial focus to the tree pane (or active modal) and
// captures the container's focus setter.
func (e *Editor) Focus(delegate func(p tview.Primitive)) {
	e.focusDelegate = delegate
	if e.hasModal() {
		e.Pages.Focus(delegate)
		return
	}
	if e.detail != nil && e.detail.HasFocus() {
		delegate(e.detail)
		return
	}
	delegate(e.tree)
}

// InputHandler handles editor-wide shortcuts before delegating to child panes.
func (e *Editor) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return e.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		e.focusDelegate = setFocus
		if e.hasModal() {
			if handler := e.Pages.InputHandler(); handler != nil {
				handler(event, setFocus)
			}
			return
		}

		switch event.Key() {
		case tcell.KeyCtrlS:
			_ = e.Save()
			return
		case tcell.KeyF6:
			switch {
			case e.tree.HasFocus():
				e.focusDetail()
			case e.activeList != nil && e.activeList.HasFocus() && e.activeItemPane != nil:
				e.setFocus(e.activeItemPane)
			default:
				e.focusTree()
			}
			return
		}

		if handler := e.Pages.InputHandler(); handler != nil {
			handler(event, setFocus)
		}
	})
}

func (e *Editor) buildLayout() {
	e.header = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)

	e.tree = tview.NewTreeView()
	e.tree.SetBorder(true).SetTitle(" Structure ")

	e.detail = tview.NewPages()
	e.detail.SetBorder(true).SetTitle(" Fields ")

	e.helpView = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetWordWrap(true)
	e.helpView.SetBorder(true).SetTitle(" Description ")

	rightPane := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(e.detail, 0, 1, false).
		AddItem(e.helpView, 6, 0, false)

	e.footer = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)

	split := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(e.tree, 0, 1, true).
		AddItem(rightPane, 0, 2, false)

	e.rootFlex = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(e.header, 1, 0, false).
		AddItem(split, 0, 1, true).
		AddItem(e.footer, 1, 0, false)

	e.AddPage(mainPageName, e.rootFlex, true, true)
}

func (e *Editor) setFocus(p tview.Primitive) {
	if e.cfg.app != nil {
		e.cfg.app.SetFocus(p)
		return
	}
	if e.focusDelegate != nil {
		e.focusDelegate(p)
	}
}

func (e *Editor) focusTree() {
	e.setFocus(e.tree)
}

func (e *Editor) focusDetail() {
	e.setFocus(e.detail)
}

func (e *Editor) hasModal() bool {
	name, _ := e.GetFrontPage()
	return name == modalPageName
}

func (e *Editor) notifyChange() {
	e.refreshTreeLabels()
	e.updateHeader()
	e.updateFooter()
	if e.cfg.onChange != nil {
		e.cfg.onChange()
	}
}

func (e *Editor) updateHeader() {
	dirtyMark := ""
	if e.doc.Dirty() {
		dirtyMark = " [yellow]*modified[-]"
	}
	if e.cfg.readOnly {
		dirtyMark += " [aqua](read-only)[-]"
	}
	path := ""
	if e.currentNode != nil {
		path = e.currentNode.Path()
	}
	e.header.SetText(fmt.Sprintf(" [::b]%s[::-]%s  [gray]%s[-]",
		tview.Escape(e.cfg.title), dirtyMark, tview.Escape(path)))
}

func (e *Editor) updateHelpView() {
	var lines []string
	if e.currentErr != nil {
		lines = append(lines, fmt.Sprintf("[red]Error: %s[-]", tview.Escape(e.currentErr.Error())))
	}
	if e.currentHelp != "" {
		lines = append(lines, "[aqua]"+tview.Escape(e.currentHelp)+"[-]")
	}
	if len(e.currentOptions) > 0 {
		lines = append(lines, fmt.Sprintf("[yellow]Available values:[-] %s",
			tview.Escape(strings.Join(e.currentOptions, ", "))))
	}
	if e.currentMeta != "" {
		lines = append(lines, "[gray]"+tview.Escape(e.currentMeta)+"[-]")
	}
	e.helpView.SetText(strings.Join(lines, "\n"))
}

func (e *Editor) updateFooter() {
	hints := []string{
		"[yellow]Tab/Enter[-] Next Pane",
		"[yellow]Esc[-] Back/Cancel",
		"[yellow]F6[-] Cycle Panes",
		"[yellow]Ctrl+S[-] Save",
	}
	if e.cfg.readOnly {
		hints = []string{
			"[yellow]Tab/Enter[-] Next Pane",
			"[yellow]Esc[-] Back/Close",
			"[yellow]F6[-] Cycle Panes",
		}
	}
	e.footer.SetText(" " + strings.Join(hints, "  |  "))
	e.updateHelpView()
}

func (e *Editor) showMessageModal(title, msg string, buttons []string, onDone func(label string)) {
	modal := tview.NewModal().
		SetText(title + "\n\n" + msg).
		AddButtons(buttons).
		SetDoneFunc(func(_ int, label string) {
			e.RemovePage(modalPageName)
			e.focusTree()
			if onDone != nil {
				onDone(label)
			}
		})
	e.AddPage(modalPageName, modal, true, true)
	e.setFocus(modal)
}

func (e *Editor) showInputModal(title, label, initial string, onSubmit func(value string) error) {
	form := tview.NewForm()
	form.SetBorder(true).SetTitle(" " + title + " ")

	val := initial
	form.AddInputField(label, initial, 40, nil, func(text string) {
		val = text
	})

	errView := tview.NewTextView().SetDynamicColors(true)

	closeModal := func() {
		e.RemovePage(modalPageName)
		e.focusDetail()
	}

	form.AddButton("OK", func() {
		if err := onSubmit(val); err != nil {
			errView.SetText("[red]" + tview.Escape(err.Error()) + "[-]")
			return
		}
		closeModal()
	})
	form.AddButton("Cancel", closeModal)
	form.SetCancelFunc(closeModal)

	box := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(form, 7, 0, true).
		AddItem(errView, 1, 0, false)

	centered := centerPrimitive(box, 60, 10)
	e.AddPage(modalPageName, centered, true, true)
	e.setFocus(form)
}

func centerPrimitive(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 0, true).
			AddItem(nil, 0, 1, false), width, 0, true).
		AddItem(nil, 0, 1, false)
}
