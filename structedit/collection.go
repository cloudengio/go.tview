// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package structedit

import (
	"fmt"
	"slices"

	"cloudeng.io/tui/tview/structmodel"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type sliceView struct {
	editor   *Editor
	node     *structmodel.Node
	tbl      *tview.Table
	actions  *tview.Form
	itemPane *tview.Pages
	readOnly bool
	fixed    bool
	lastRow  int
}

func (e *Editor) buildSliceView(mn *structmodel.Node) tview.Primitive {
	sv := &sliceView{
		editor:   e,
		node:     mn,
		tbl:      newCollectionTable(),
		actions:  tview.NewForm(),
		itemPane: tview.NewPages(),
		readOnly: e.cfg.readOnly || mn.ReadOnly(),
		fixed:    mn.IsFixedArray(),
		lastRow:  -1,
	}

	title := " Items "
	if !sv.readOnly && !sv.fixed {
		title = " Items [a:Add d:Del u/j:Move] "
	}
	sv.tbl.SetBorder(true).SetTitle(title)
	sv.itemPane.SetBorder(true).SetTitle(" Selected Item ")
	sv.actions.SetHorizontal(true)

	populateSliceTable(sv.tbl, mn)
	sv.addButtons()
	sv.bindEvents()
	sv.showItem(sv.selectedIdx())
	e.activeList = sv.tbl
	e.activeItemPane = sv.itemPane

	left := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(sv.tbl, 0, 1, true).
		AddItem(sv.actions, 3, 0, false)

	return tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(left, 0, 2, true).
		AddItem(sv.itemPane, 0, 3, false)
}

func (sv *sliceView) selectedIdx() int {
	if sv.node.Len() == 0 {
		return -1
	}
	row, _ := sv.tbl.GetSelection()
	if row < 1 || row > sv.node.Len() {
		return 0
	}
	return row - 1
}

func (sv *sliceView) refresh(selectRow int) {
	populateSliceTable(sv.tbl, sv.node)
	if sv.node.Len() > 0 {
		selectRow = min(max(selectRow, 1), sv.node.Len())
		sv.tbl.Select(selectRow, 0)
		sv.showItem(selectRow - 1)
	} else {
		sv.showItem(-1)
	}
	sv.editor.refreshCurrentTreeBranch()
	sv.editor.notifyChange()
}

func (sv *sliceView) updateRowSummary(i int) {
	if i < 0 || i >= sv.node.Len() {
		return
	}
	sv.tbl.SetCell(i+1, 1, tview.NewTableCell(sv.node.ItemSummary(i)).SetExpansion(1))
	sv.editor.refreshCurrentTreeBranch()
	sv.editor.notifyChange()
}

func (sv *sliceView) showItem(i int) {
	sv.lastRow = i
	sv.editor.activeForm = nil
	sv.editor.activeBindings = nil

	if i < 0 || i >= sv.node.Len() {
		sv.itemPane.SetTitle(" Selected Item ")
		msg := "No items in list."
		if !sv.readOnly && !sv.fixed {
			msg = "No items in list.\n\nPress 'a' or select Add to create an item."
		}
		sv.itemPane.AddAndSwitchToPage("item", emptyItemView(msg, func() { sv.editor.setFocus(sv.tbl) }), true)
		return
	}

	sv.itemPane.SetTitle(fmt.Sprintf(" Item [%d] ", i))
	if sv.node.ElemIsScalar() {
		view := sv.buildScalarItemForm(i)
		sv.itemPane.AddAndSwitchToPage("item", view, true)
		return
	}

	child := sv.node.ChildAt(i)
	if child == nil {
		sv.itemPane.AddAndSwitchToPage("item", emptyItemView("Unable to inspect item.", func() { sv.editor.setFocus(sv.tbl) }), true)
		return
	}
	view := sv.editor.buildCollectionItemDetail(
		child,
		func() { sv.editor.setFocus(sv.tbl) },
		func() { sv.updateRowSummary(i) },
		func() { sv.showItem(i) },
	)
	sv.itemPane.AddAndSwitchToPage("item", view, true)
}

func (sv *sliceView) buildScalarItemForm(i int) tview.Primitive {
	cur, _ := sv.node.ItemScalar(i)
	return sv.editor.buildScalarEditorForm(
		"Value",
		cur,
		sv.node.ElemEnumOptions(),
		sv.readOnly,
		func(val string) error {
			if err := sv.node.SetItemScalar(i, val); err != nil {
				return err
			}
			sv.updateRowSummary(i)
			return nil
		},
		func() { sv.editor.setFocus(sv.tbl) },
	)
}

func (sv *sliceView) addItem() {
	if sv.readOnly || sv.fixed {
		return
	}
	idx, err := sv.node.AppendItem()
	sv.handleOpResult(err, idx+1)
}

func (sv *sliceView) dupItem() {
	if sv.readOnly || sv.fixed {
		return
	}
	i := sv.selectedIdx()
	if i < 0 {
		return
	}
	idx, err := sv.node.DuplicateItem(i)
	sv.handleOpResult(err, idx+1)
}

func (sv *sliceView) moveItem(delta int) {
	if sv.readOnly {
		return
	}
	i := sv.selectedIdx()
	target := i + delta
	if i < 0 || target < 0 || target >= sv.node.Len() {
		return
	}
	sv.handleOpResult(sv.node.MoveItem(i, target), target+1)
}

func (sv *sliceView) delItem() {
	if sv.readOnly || sv.fixed {
		return
	}
	i := sv.selectedIdx()
	if i < 0 {
		return
	}
	sv.handleOpResult(sv.node.DeleteItem(i), i+1)
}

func (sv *sliceView) handleOpResult(err error, selectRow int) {
	if err != nil {
		sv.editor.currentErr = err
		sv.editor.updateFooter()
		return
	}
	sv.editor.currentErr = nil
	sv.refresh(selectRow)
}

func (sv *sliceView) bindEvents() {
	sv.tbl.SetSelectionChangedFunc(func(row, _ int) {
		idx := row - 1
		if sv.node.Len() > 0 && idx != sv.lastRow {
			sv.showItem(idx)
		}
	})
	sv.tbl.SetSelectedFunc(func(_, _ int) {
		if sv.node.Len() > 0 {
			sv.editor.setFocus(sv.itemPane)
		}
	})
	sv.tbl.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEscape:
			sv.editor.focusTree()
		case tcell.KeyTab:
			sv.editor.setFocus(sv.actions)
		case tcell.KeyBacktab:
			sv.editor.focusTree()
		}
	})
	sv.tbl.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyRight:
			if sv.node.Len() > 0 {
				sv.editor.setFocus(sv.itemPane)
				return nil
			}
		case tcell.KeyLeft:
			sv.editor.focusTree()
			return nil
		}
		switch event.Rune() {
		case 'a':
			sv.addItem()
			return nil
		case 'd':
			sv.delItem()
			return nil
		case 'c':
			sv.dupItem()
			return nil
		case 'u':
			sv.moveItem(-1)
			return nil
		case 'j':
			sv.moveItem(1)
			return nil
		}
		return event
	})

	bindActionBarKeys(sv.editor, sv.actions, sv.tbl, sv.itemPane)
}

func (sv *sliceView) addButtons() {
	if !sv.readOnly && !sv.fixed {
		sv.actions.AddButton("Add", sv.addItem)
		sv.actions.AddButton("Dup", sv.dupItem)
	}
	if !sv.readOnly {
		sv.actions.AddButton("Up", func() { sv.moveItem(-1) })
		sv.actions.AddButton("Dn", func() { sv.moveItem(1) })
		if !sv.fixed {
			sv.actions.AddButton("Del", sv.delItem)
		}
	}
	sv.editor.addPtrClearButton(sv.actions, sv.node, sv.readOnly)
}

type mapView struct {
	editor   *Editor
	node     *structmodel.Node
	tbl      *tview.Table
	actions  *tview.Form
	itemPane *tview.Pages
	keys     []string
	readOnly bool
	lastKey  string
}

func (e *Editor) buildMapView(mn *structmodel.Node) tview.Primitive {
	mv := &mapView{
		editor:   e,
		node:     mn,
		tbl:      newCollectionTable(),
		actions:  tview.NewForm(),
		itemPane: tview.NewPages(),
		readOnly: e.cfg.readOnly || mn.ReadOnly(),
	}

	title := " Keys "
	if !mv.readOnly {
		title = " Keys [a:Add r:Ren d:Del] "
	}
	mv.tbl.SetBorder(true).SetTitle(title)
	mv.itemPane.SetBorder(true).SetTitle(" Selected Entry ")
	mv.actions.SetHorizontal(true)

	mv.keys = populateMapTable(mv.tbl, mn)
	mv.addButtons()
	mv.bindEvents()
	if k, ok := mv.selectedKey(); ok {
		mv.showEntry(k)
	} else {
		mv.showEntry("")
	}
	e.activeList = mv.tbl
	e.activeItemPane = mv.itemPane

	left := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(mv.tbl, 0, 1, true).
		AddItem(mv.actions, 3, 0, false)

	return tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(left, 0, 2, true).
		AddItem(mv.itemPane, 0, 3, false)
}

func (mv *mapView) selectedKey() (string, bool) {
	if len(mv.keys) == 0 {
		return "", false
	}
	row, _ := mv.tbl.GetSelection()
	if row < 1 || row > len(mv.keys) {
		return mv.keys[0], true
	}
	return mv.keys[row-1], true
}

func (mv *mapView) refresh(focusKey string) {
	mv.keys = populateMapTable(mv.tbl, mv.node)
	selected := ""
	if len(mv.keys) > 0 {
		selected = mv.keys[0]
		for i, k := range mv.keys {
			if k == focusKey {
				mv.tbl.Select(i+1, 0)
				selected = k
				break
			}
		}
	}
	mv.showEntry(selected)
	mv.editor.refreshCurrentTreeBranch()
	mv.editor.notifyChange()
}

func (mv *mapView) updateKeySummary(key string) {
	for i, k := range mv.keys {
		if k == key {
			mv.tbl.SetCell(i+1, 1, tview.NewTableCell(mv.node.MapEntrySummary(k)).SetExpansion(1))
			break
		}
	}
	mv.editor.refreshCurrentTreeBranch()
	mv.editor.notifyChange()
}

func (mv *mapView) showEntry(key string) {
	mv.lastKey = key
	mv.editor.activeForm = nil
	mv.editor.activeBindings = nil

	if key == "" {
		mv.itemPane.SetTitle(" Selected Entry ")
		msg := "No entries in map."
		if !mv.readOnly {
			msg = "No entries in map.\n\nPress 'a' or select Add to create an entry."
		}
		mv.itemPane.AddAndSwitchToPage("item", emptyItemView(msg, func() { mv.editor.setFocus(mv.tbl) }), true)
		return
	}

	mv.itemPane.SetTitle(fmt.Sprintf(" Entry [%s] ", key))
	if mv.node.ElemIsScalar() {
		view := mv.editor.buildScalarEditorForm(
			"Value",
			mv.node.MapEntrySummary(key),
			mv.node.ElemEnumOptions(),
			mv.readOnly,
			func(val string) error {
				if err := mv.node.SetMapEntryScalar(key, val); err != nil {
					return err
				}
				mv.updateKeySummary(key)
				return nil
			},
			func() { mv.editor.setFocus(mv.tbl) },
		)
		mv.itemPane.AddAndSwitchToPage("item", view, true)
		return
	}

	child := mv.node.MapChild(key)
	if child == nil {
		mv.itemPane.AddAndSwitchToPage("item", emptyItemView("Unable to inspect entry.", func() { mv.editor.setFocus(mv.tbl) }), true)
		return
	}
	view := mv.editor.buildCollectionItemDetail(
		child,
		func() { mv.editor.setFocus(mv.tbl) },
		func() { mv.updateKeySummary(key) },
		func() { mv.showEntry(key) },
	)
	mv.itemPane.AddAndSwitchToPage("item", view, true)
}

func (mv *mapView) addEntry() {
	if mv.readOnly {
		return
	}
	mv.editor.showInputModal("Add Map Entry", "Key", "", func(k string) error {
		if err := mv.node.AddMapEntry(k); err != nil {
			return err
		}
		mv.refresh(k)
		return nil
	})
}

func (mv *mapView) renameEntry() {
	if mv.readOnly {
		return
	}
	oldKey, ok := mv.selectedKey()
	if !ok {
		return
	}
	mv.editor.showInputModal("Rename Map Key", "New Key", oldKey, func(nk string) error {
		if err := mv.node.RenameMapKey(oldKey, nk); err != nil {
			return err
		}
		mv.refresh(nk)
		return nil
	})
}

func (mv *mapView) delEntry() {
	if mv.readOnly {
		return
	}
	k, ok := mv.selectedKey()
	if !ok {
		return
	}
	if err := mv.node.DeleteMapEntry(k); err != nil {
		mv.editor.currentErr = err
		mv.editor.updateFooter()
		return
	}
	mv.editor.currentErr = nil
	mv.refresh("")
}

func (mv *mapView) bindEvents() {
	mv.tbl.SetSelectionChangedFunc(func(row, _ int) {
		idx := row - 1
		if idx >= 0 && idx < len(mv.keys) && mv.keys[idx] != mv.lastKey {
			mv.showEntry(mv.keys[idx])
		}
	})
	mv.tbl.SetSelectedFunc(func(_, _ int) {
		if len(mv.keys) > 0 {
			mv.editor.setFocus(mv.itemPane)
		}
	})
	mv.tbl.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEscape:
			mv.editor.focusTree()
		case tcell.KeyTab:
			mv.editor.setFocus(mv.actions)
		case tcell.KeyBacktab:
			mv.editor.focusTree()
		}
	})
	mv.tbl.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyRight:
			if len(mv.keys) > 0 {
				mv.editor.setFocus(mv.itemPane)
				return nil
			}
		case tcell.KeyLeft:
			mv.editor.focusTree()
			return nil
		}
		switch event.Rune() {
		case 'a':
			mv.addEntry()
			return nil
		case 'r':
			mv.renameEntry()
			return nil
		case 'd':
			mv.delEntry()
			return nil
		}
		return event
	})

	bindActionBarKeys(mv.editor, mv.actions, mv.tbl, mv.itemPane)
}

func (mv *mapView) addButtons() {
	if !mv.readOnly {
		mv.actions.AddButton("Add", mv.addEntry)
		mv.actions.AddButton("Ren", mv.renameEntry)
		mv.actions.AddButton("Del", mv.delEntry)
	}
	mv.editor.addPtrClearButton(mv.actions, mv.node, mv.readOnly)
}

func (e *Editor) buildCollectionItemDetail(
	child *structmodel.Node,
	onBack func(),
	onEdited func(),
	onReRender func(),
) tview.Primitive {
	if child.HasCycle() {
		return e.buildCycleView(child)
	}
	if child.IsPointer() && child.IsNil() {
		form := tview.NewForm()
		form.SetCancelFunc(onBack)
		form.AddTextView("Status", fmt.Sprintf("nil (%s)", child.TargetTypeName()), 0, 1, false, false)
		if !e.cfg.readOnly && !child.ReadOnly() {
			form.AddButton("Initialize", func() {
				if err := child.InitPtr(); err != nil {
					e.currentErr = err
					e.updateFooter()
					return
				}
				e.currentErr = nil
				onEdited()
				onReRender()
			})
		}
		return form
	}

	drillDown := func() { e.navigateToChildPath(child.Path()) }

	switch child.TargetKind() {
	case structmodel.KindSlice, structmodel.KindArray, structmodel.KindMap:
		form := tview.NewForm()
		form.SetCancelFunc(onBack)
		form.AddTextView("Summary", fmt.Sprintf("%s %s", child.TargetTypeName(), child.Summary()), 0, 1, false, false)
		form.AddButton("Open in Tree", drillDown)
		return form
	default:
		return e.buildNodeForm(child, onBack, onEdited, drillDown)
	}
}

func (e *Editor) buildScalarEditorForm(
	label string,
	initial string,
	options []string,
	readOnly bool,
	onSet func(string) error,
	onBack func(),
) tview.Primitive {
	form := tview.NewForm()
	form.SetCancelFunc(onBack)

	handleChange := func(val string) {
		if err := onSet(val); err != nil {
			e.currentErr = err
			e.updateFooter()
			return
		}
		e.currentErr = nil
		e.updateFooter()
	}

	if len(options) > 0 {
		idx := slices.Index(options, initial)
		dd := tview.NewDropDown().
			SetLabel(label).
			SetUseStyleTags(false).
			SetTextOptions(" ", " ", "", " ▾", "(select)").
			SetOptions(options, nil)
		if idx >= 0 {
			dd.SetCurrentOption(idx)
		}
		dd.SetSelectedFunc(func(text string, _ int) { handleChange(text) })
		dd.SetDisabled(readOnly)
		form.AddFormItem(dd)
		return form
	}

	in := tview.NewInputField().
		SetLabel(label).
		SetText(initial).
		SetFieldWidth(40)
	in.SetChangedFunc(handleChange)
	in.SetDisabled(readOnly)
	form.AddFormItem(in)
	return form
}

func bindActionBarKeys(e *Editor, actions *tview.Form, tbl *tview.Table, itemPane tview.Primitive) {
	actions.SetCancelFunc(func() { e.setFocus(tbl) })
	actions.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		_, btn := actions.GetFocusedItemIndex()
		count := actions.GetButtonCount()
		switch event.Key() {
		case tcell.KeyUp:
			e.setFocus(tbl)
			return nil
		case tcell.KeyTab:
			if count == 0 || btn >= count-1 {
				e.setFocus(itemPane)
				return nil
			}
		case tcell.KeyBacktab:
			if btn <= 0 {
				e.setFocus(tbl)
				return nil
			}
		}
		return event
	})
}

func emptyItemView(msg string, onBack func()) tview.Primitive {
	tv := tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetText("\n " + msg)
	tv.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape && onBack != nil {
			onBack()
		}
	})
	return tv
}

func (e *Editor) addPtrClearButton(actions *tview.Form, mn *structmodel.Node, readOnly bool) {
	if mn.IsPointer() && !readOnly {
		actions.AddButton("Clr", func() {
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
}

func newCollectionTable() *tview.Table {
	tbl := tview.NewTable().
		SetFixed(1, 0).
		SetSelectable(true, false).
		SetBorders(false)
	tbl.SetSelectedStyle(tcell.StyleDefault.
		Background(tcell.ColorDarkCyan).
		Foreground(tcell.ColorWhite))
	return tbl
}

func populateSliceTable(tbl *tview.Table, mn *structmodel.Node) {
	tbl.Clear()
	tbl.SetCell(0, 0, tview.NewTableCell("#").
		SetSelectable(false).
		SetTextColor(tcell.ColorYellow))
	tbl.SetCell(0, 1, tview.NewTableCell("Summary").
		SetSelectable(false).
		SetExpansion(1).
		SetTextColor(tcell.ColorYellow))

	n := mn.Len()
	if n == 0 {
		tbl.SetCell(1, 0, tview.NewTableCell("-").SetSelectable(false).SetTextColor(tcell.ColorGray))
		tbl.SetCell(1, 1, tview.NewTableCell("(empty)").SetSelectable(false).SetTextColor(tcell.ColorGray))
		return
	}
	for i := 0; i < n; i++ {
		tbl.SetCell(i+1, 0, tview.NewTableCell(fmt.Sprintf("%d", i)))
		tbl.SetCell(i+1, 1, tview.NewTableCell(mn.ItemSummary(i)).SetExpansion(1))
	}
}

func populateMapTable(tbl *tview.Table, mn *structmodel.Node) []string {
	tbl.Clear()
	tbl.SetCell(0, 0, tview.NewTableCell("Key").
		SetSelectable(false).
		SetTextColor(tcell.ColorYellow))
	tbl.SetCell(0, 1, tview.NewTableCell("Value / Summary").
		SetSelectable(false).
		SetExpansion(1).
		SetTextColor(tcell.ColorYellow))

	keys := mn.MapKeys()
	if len(keys) == 0 {
		tbl.SetCell(1, 0, tview.NewTableCell("-").SetSelectable(false).SetTextColor(tcell.ColorGray))
		tbl.SetCell(1, 1, tview.NewTableCell("(empty)").SetSelectable(false).SetTextColor(tcell.ColorGray))
		return nil
	}
	for i, k := range keys {
		tbl.SetCell(i+1, 0, tview.NewTableCell(k))
		tbl.SetCell(i+1, 1, tview.NewTableCell(mn.MapEntrySummary(k)).SetExpansion(1))
	}
	return keys
}
