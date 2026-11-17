// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package structedit

import (
	"fmt"

	"cloudeng.io/tui/tview/structmodel"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type sliceView struct {
	editor   *Editor
	node     *structmodel.Node
	tbl      *tview.Table
	actions  *tview.Form
	readOnly bool
	fixed    bool
}

func (e *Editor) buildSliceView(mn *structmodel.Node) tview.Primitive {
	sv := &sliceView{
		editor:   e,
		node:     mn,
		tbl:      newCollectionTable(),
		actions:  tview.NewForm(),
		readOnly: e.cfg.readOnly || mn.ReadOnly(),
		fixed:    mn.IsFixedArray(),
	}
	sv.actions.SetHorizontal(true)
	sv.actions.SetCancelFunc(func() { e.focusTree() })
	populateSliceTable(sv.tbl, mn)
	sv.bindEvents()
	sv.addButtons()

	return tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(sv.tbl, 0, 1, true).
		AddItem(sv.actions, 3, 0, false)
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
	}
	sv.editor.refreshCurrentTreeBranch()
	sv.editor.notifyChange()
}

func (sv *sliceView) editSelected() {
	i := sv.selectedIdx()
	if i < 0 {
		return
	}
	if !sv.node.ElemIsScalar() {
		sv.editor.navigateToChildPath(fmt.Sprintf("%s[%d]", sv.node.Path(), i))
		return
	}
	if sv.readOnly {
		return
	}
	cur, _ := sv.node.ItemScalar(i)
	sv.editor.showInputModal("Edit Item", fmt.Sprintf("[%d]", i), cur, func(val string) error {
		if err := sv.node.SetItemScalar(i, val); err != nil {
			return err
		}
		sv.refresh(i + 1)
		return nil
	})
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
	sv.tbl.SetSelectedFunc(func(_, _ int) { sv.editSelected() })
	sv.tbl.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEscape:
			sv.editor.focusTree()
		case tcell.KeyTab:
			sv.editor.setFocus(sv.actions)
		}
	})
	sv.tbl.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
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
}

func (sv *sliceView) addButtons() {
	if !sv.readOnly && !sv.fixed {
		sv.actions.AddButton("Add", sv.addItem)
	}
	sv.actions.AddButton("Edit", sv.editSelected)
	if !sv.readOnly {
		if !sv.fixed {
			sv.actions.AddButton("Duplicate", sv.dupItem)
		}
		sv.actions.AddButton("Up", func() { sv.moveItem(-1) })
		sv.actions.AddButton("Down", func() { sv.moveItem(1) })
		if !sv.fixed {
			sv.actions.AddButton("Delete", sv.delItem)
		}
	}
	sv.editor.addPtrClearButton(sv.actions, sv.node, sv.readOnly)
}

type mapView struct {
	editor   *Editor
	node     *structmodel.Node
	tbl      *tview.Table
	actions  *tview.Form
	keys     []string
	readOnly bool
}

func (e *Editor) buildMapView(mn *structmodel.Node) tview.Primitive {
	mv := &mapView{
		editor:   e,
		node:     mn,
		tbl:      newCollectionTable(),
		actions:  tview.NewForm(),
		readOnly: e.cfg.readOnly || mn.ReadOnly(),
	}
	mv.actions.SetHorizontal(true)
	mv.actions.SetCancelFunc(func() { e.focusTree() })
	mv.keys = populateMapTable(mv.tbl, mn)
	mv.bindEvents()
	mv.addButtons()

	return tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(mv.tbl, 0, 1, true).
		AddItem(mv.actions, 3, 0, false)
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
	for i, k := range mv.keys {
		if k == focusKey {
			mv.tbl.Select(i+1, 0)
			break
		}
	}
	mv.editor.refreshCurrentTreeBranch()
	mv.editor.notifyChange()
}

func (mv *mapView) editSelected() {
	k, ok := mv.selectedKey()
	if !ok {
		return
	}
	if !mv.node.ElemIsScalar() {
		mv.editor.navigateToChildPath(fmt.Sprintf("%s[%s]", mv.node.Path(), k))
		return
	}
	if mv.readOnly {
		return
	}
	cur := mv.node.MapEntrySummary(k)
	mv.editor.showInputModal("Edit Value", k, cur, func(val string) error {
		if err := mv.node.SetMapEntryScalar(k, val); err != nil {
			return err
		}
		mv.refresh(k)
		return nil
	})
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
	mv.tbl.SetSelectedFunc(func(_, _ int) { mv.editSelected() })
	mv.tbl.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEscape:
			mv.editor.focusTree()
		case tcell.KeyTab:
			mv.editor.setFocus(mv.actions)
		}
	})
	mv.tbl.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
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
}

func (mv *mapView) addButtons() {
	if !mv.readOnly {
		mv.actions.AddButton("Add", mv.addEntry)
		mv.actions.AddButton("Rename", mv.renameEntry)
	}
	mv.actions.AddButton("Edit", mv.editSelected)
	if !mv.readOnly {
		mv.actions.AddButton("Delete", mv.delEntry)
	}
	mv.editor.addPtrClearButton(mv.actions, mv.node, mv.readOnly)
}

func (e *Editor) addPtrClearButton(actions *tview.Form, mn *structmodel.Node, readOnly bool) {
	if mn.IsPointer() && !readOnly {
		actions.AddButton("Clear (set nil)", func() {
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
