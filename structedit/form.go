// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package structedit

import (
	"fmt"
	"slices"

	"cloudeng.io/tui/tview/structmodel"
	"cloudeng.io/tui/tview/structtag"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	detailPageName   = "detail"
	fixedMaskDisplay = "********"
)

func (e *Editor) renderDetail(mn *structmodel.Node) {
	var view tview.Primitive
	switch {
	case mn.HasCycle():
		view = e.buildCycleView(mn)
	case mn.IsPointer() && mn.IsNil():
		view = e.buildNilPtrView(mn)
	default:
		switch mn.TargetKind() {
		case structmodel.KindSlice, structmodel.KindArray:
			view = e.buildSliceView(mn)
		case structmodel.KindMap:
			view = e.buildMapView(mn)
		default:
			view = e.buildFormView(mn)
		}
	}
	e.detail.AddAndSwitchToPage(detailPageName, view, true)
}

func (e *Editor) buildCycleView(mn *structmodel.Node) tview.Primitive {
	tv := tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetText(fmt.Sprintf(
			"\n [yellow]Recursive pointer cycle detected[-]\n\n"+
				" Field [::b]%s[::-] (%s) points back to an ancestor node already open above.\n\n"+
				" Edit the ancestor node directly to modify its fields.\n\n"+
				" Press [yellow]Esc[-] to return to the tree.",
			tview.Escape(mn.Path()), tview.Escape(mn.TargetTypeName())))
	tv.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			e.focusTree()
		}
	})
	return tv
}

func (e *Editor) buildNilPtrView(mn *structmodel.Node) tview.Primitive {
	form := tview.NewForm()
	form.SetCancelFunc(func() { e.focusTree() })
	form.AddTextView("Status", fmt.Sprintf("nil (%s)", mn.TargetTypeName()), 0, 1, false, false)

	if !e.cfg.readOnly && !mn.ReadOnly() {
		form.AddButton("Initialize", func() {
			if err := mn.InitPtr(); err != nil {
				e.currentErr = err
				e.updateFooter()
				return
			}
			e.currentErr = nil
			e.refreshCurrentTreeBranch()
			e.renderDetail(mn)
			e.notifyChange()
			e.focusDetail()
		})
	}
	return form
}

func (e *Editor) buildFormView(mn *structmodel.Node) tview.Primitive {
	fields := mn.ScalarFields()
	form := tview.NewForm()
	form.SetCancelFunc(func() { e.focusTree() })

	if len(fields) == 0 {
		msg := "No scalar fields on this node. Use the Structure tree to edit child sections."
		if len(mn.Children()) == 0 {
			msg = "No editable fields on this node."
		}
		form.AddTextView("Info", msg, 0, 2, false, true)
	}

	var itemBindings []*structmodel.FieldBinding
	for _, fb := range fields {
		items := e.buildFormItems(fb)
		for _, item := range items {
			form.AddFormItem(item)
			itemBindings = append(itemBindings, fb)
		}
	}

	e.addFormActions(form, mn)

	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		e.syncFocusedFieldHelp(form, itemBindings)
		return event
	})
	return form
}

func (e *Editor) addFormActions(form *tview.Form, mn *structmodel.Node) {
	if mn.IsPointer() && !e.cfg.readOnly && !mn.ReadOnly() {
		form.AddButton("Clear (set nil)", func() {
			if err := mn.ClearPtr(); err != nil {
				e.currentErr = err
				e.updateFooter()
				return
			}
			e.currentErr = nil
			e.refreshCurrentTreeBranch()
			e.renderDetail(mn)
			e.notifyChange()
			e.focusTree()
		})
	}
	if mn == e.doc.Root() && !e.cfg.readOnly {
		form.AddButton("Save", func() { _ = e.Save() })
		form.AddButton("Revert", func() { e.Revert() })
	}
}

func (e *Editor) syncFocusedFieldHelp(form *tview.Form, bindings []*structmodel.FieldBinding) {
	idx, _ := form.GetFocusedItemIndex()
	if idx >= 0 && idx < len(bindings) {
		fb := bindings[idx]
		e.currentHelp = formatFieldHelp(fb)
		e.currentErr = fb.Error()
		e.updateFooter()
	}
}

func formatFieldHelp(fb *structmodel.FieldBinding) string {
	tag := fb.Tag()
	base := fmt.Sprintf("key: %s (%s)", tag.Name, fb.Type())
	if tag.Help != "" {
		base = fmt.Sprintf("%s (key: %s)", tag.Help, tag.Name)
	}
	if tag.Mask == structtag.MaskHideWithShow {
		return base + " [Ctrl+R or checkbox to show/hide]"
	}
	return base
}

func (e *Editor) buildFormItems(fb *structmodel.FieldBinding) []tview.FormItem {
	tag := fb.Tag()
	disabled := e.cfg.readOnly || fb.ReadOnly()

	if factory, ok := e.cfg.namedWidget[tag.Widget]; ok {
		item := factory(fb, func() { e.onFieldEdited(fb, nil) })
		item.SetDisabled(disabled)
		return []tview.FormItem{item}
	}
	if factory, ok := e.cfg.typeWidget[fb.Type()]; ok {
		item := factory(fb, func() { e.onFieldEdited(fb, nil) })
		item.SetDisabled(disabled)
		return []tview.FormItem{item}
	}

	if len(tag.Options) > 0 || tag.Widget == "dropdown" {
		item := e.buildDropDownItem(fb)
		item.SetDisabled(disabled)
		return []tview.FormItem{item}
	}
	if fb.ScalarKind() == structmodel.ScalarBool && tag.Widget == "" {
		item := e.buildCheckboxItem(fb)
		item.SetDisabled(disabled)
		return []tview.FormItem{item}
	}
	if tag.Widget == "textarea" || (fb.IsSliceString() && tag.Widget == "lines") {
		item := e.buildTextAreaItem(fb)
		item.SetDisabled(disabled)
		return []tview.FormItem{item}
	}

	switch tag.Mask {
	case structtag.MaskAlwaysHide:
		item := e.buildAlwaysHiddenItem(fb, disabled)
		item.SetDisabled(disabled)
		return []tview.FormItem{item}
	case structtag.MaskHideWithShow:
		return e.buildHideWithShowItems(fb, disabled)
	default:
		item := e.buildInputItem(fb)
		item.SetDisabled(disabled)
		return []tview.FormItem{item}
	}
}

func (e *Editor) buildCheckboxItem(fb *structmodel.FieldBinding) tview.FormItem {
	cb := tview.NewCheckbox().
		SetLabel(fb.Tag().Label).
		SetChecked(fb.Bool())
	cb.SetChangedFunc(func(checked bool) {
		err := fb.SetBool(checked)
		e.onFieldEdited(fb, err)
	})
	return cb
}

func (e *Editor) buildDropDownItem(fb *structmodel.FieldBinding) tview.FormItem {
	opts := fb.Tag().Options
	curVal := fb.String()
	initial := slices.Index(opts, curVal)
	dd := tview.NewDropDown().
		SetLabel(fb.Tag().Label).
		SetOptions(opts, nil)
	if initial >= 0 {
		dd.SetCurrentOption(initial)
	}
	dd.SetSelectedFunc(func(text string, _ int) {
		err := fb.SetString(text)
		e.onFieldEdited(fb, err)
	})
	return dd
}

func (e *Editor) buildTextAreaItem(fb *structmodel.FieldBinding) tview.FormItem {
	ta := tview.NewTextArea().
		SetLabel(fb.Tag().Label).
		SetText(fb.String(), false).
		SetSize(4, 48)
	if ph := fb.Tag().Placeholder; ph != "" {
		ta.SetPlaceholder(ph)
	}
	ta.SetChangedFunc(func() {
		err := fb.SetString(ta.GetText())
		e.onFieldEdited(fb, err)
	})
	return ta
}

func (e *Editor) buildInputItem(fb *structmodel.FieldBinding) tview.FormItem {
	tag := fb.Tag()
	in := tview.NewInputField().
		SetLabel(tag.Label).
		SetText(fb.String()).
		SetFieldWidth(40)
	if tag.Placeholder != "" {
		in.SetPlaceholder(tag.Placeholder)
	}
	in.SetChangedFunc(func(text string) {
		err := fb.SetString(text)
		e.onFieldEdited(fb, err)
	})
	return in
}

type alwaysHiddenInput struct {
	*tview.InputField
	onBlur func()
}

func (a *alwaysHiddenInput) Blur() {
	a.InputField.Blur()
	if a.onBlur != nil {
		a.onBlur()
	}
}

func (e *Editor) buildAlwaysHiddenItem(fb *structmodel.FieldBinding, disabled bool) tview.FormItem {
	tag := fb.Tag()
	in := tview.NewInputField().
		SetLabel(tag.Label).
		SetFieldWidth(40)

	editing := false
	suppress := false
	resetDisplay := func() {
		suppress = true
		in.SetMaskCharacter(0)
		in.SetText(fixedMaskDisplay)
		suppress = false
		editing = false
	}
	resetDisplay()

	in.SetChangedFunc(func(text string) {
		if suppress || !editing {
			return
		}
		err := fb.SetString(text)
		e.onFieldEdited(fb, err)
	})
	in.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if disabled || editing {
			return event
		}
		switch event.Key() {
		case tcell.KeyRune, tcell.KeyBackspace, tcell.KeyBackspace2, tcell.KeyDelete:
			editing = true
			suppress = true
			in.SetText("")
			in.SetMaskCharacter('*')
			suppress = false
			if event.Key() != tcell.KeyRune {
				return nil
			}
		}
		return event
	})

	return &alwaysHiddenInput{
		InputField: in,
		onBlur:     resetDisplay,
	}
}

func (e *Editor) buildHideWithShowItems(fb *structmodel.FieldBinding, disabled bool) []tview.FormItem {
	tag := fb.Tag()
	in := tview.NewInputField().
		SetLabel(tag.Label).
		SetText(fb.String()).
		SetFieldWidth(40).
		SetMaskCharacter('*')
	if tag.Placeholder != "" {
		in.SetPlaceholder(tag.Placeholder)
	}
	in.SetDisabled(disabled)
	in.SetChangedFunc(func(text string) {
		err := fb.SetString(text)
		e.onFieldEdited(fb, err)
	})

	revealed := false
	showCB := tview.NewCheckbox().
		SetLabel("Show " + tag.Label).
		SetChecked(false)

	setRevealed := func(show bool) {
		revealed = show
		if revealed {
			in.SetMaskCharacter(0)
		} else {
			in.SetMaskCharacter('*')
		}
		if showCB.IsChecked() != revealed {
			showCB.SetChecked(revealed)
		}
	}

	showCB.SetChangedFunc(func(checked bool) {
		if checked != revealed {
			setRevealed(checked)
		}
	})
	in.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlR {
			setRevealed(!revealed)
			return nil
		}
		return event
	})

	return []tview.FormItem{in, showCB}
}

func (e *Editor) onFieldEdited(fb *structmodel.FieldBinding, err error) {
	e.currentHelp = formatFieldHelp(fb)
	e.currentErr = err
	e.notifyChange()
}
