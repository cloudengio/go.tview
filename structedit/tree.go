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

func (e *Editor) initTree() {
	rootModel := e.doc.Root()
	rootTN := e.makeTreeNode(rootModel)
	rootTN.SetExpanded(true)
	e.populateTreeChildren(rootTN, rootModel, true)

	e.tree.SetRoot(rootTN).
		SetCurrentNode(rootTN).
		SetTopLevel(0)

	e.tree.SetChangedFunc(func(tn *tview.TreeNode) {
		if mn, ok := tn.GetReference().(*structmodel.Node); ok {
			e.selectModelNode(mn, false)
		}
	})

	e.tree.SetSelectedFunc(func(tn *tview.TreeNode) {
		mn, ok := tn.GetReference().(*structmodel.Node)
		if !ok {
			return
		}
		if !tn.IsExpanded() && len(mn.Children()) > 0 {
			tn.SetExpanded(true)
			e.populateTreeChildren(tn, mn, false)
		}
		e.selectModelNode(mn, true)
	})

	e.tree.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			e.Cancel()
		}
	})

	e.tree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		cur := e.tree.GetCurrentNode()
		if cur == nil {
			return event
		}
		mn, ok := cur.GetReference().(*structmodel.Node)
		if !ok {
			return event
		}
		switch event.Key() {
		case tcell.KeyRight:
			if !mn.HasCycle() && len(mn.Children()) > 0 {
				e.populateTreeChildren(cur, mn, false)
				cur.SetExpanded(true)
			}
			return nil
		case tcell.KeyLeft:
			if cur.IsExpanded() {
				cur.SetExpanded(false)
				return nil
			}
		}
		return event
	})
}

func (e *Editor) makeTreeNode(mn *structmodel.Node) *tview.TreeNode {
	tn := tview.NewTreeNode(formatTreeNodeText(mn)).
		SetReference(mn).
		SetSelectable(true)
	applyTreeNodeColor(tn, mn)
	return tn
}

func (e *Editor) populateTreeChildren(tn *tview.TreeNode, mn *structmodel.Node, recurseTaggedExpand bool) {
	tn.ClearChildren()
	for _, ch := range mn.Children() {
		childTN := e.makeTreeNode(ch)
		expand := recurseTaggedExpand && ch.Tag().Expand && !ch.HasCycle()
		childTN.SetExpanded(expand)
		if expand {
			e.populateTreeChildren(childTN, ch, true)
		}
		tn.AddChild(childTN)
	}
}

func (e *Editor) refreshCurrentTreeBranch() {
	cur := e.tree.GetCurrentNode()
	if cur == nil {
		return
	}
	mn, ok := cur.GetReference().(*structmodel.Node)
	if !ok {
		return
	}
	cur.SetText(formatTreeNodeText(mn))
	applyTreeNodeColor(cur, mn)
	if cur.IsExpanded() || len(cur.GetChildren()) > 0 {
		e.populateTreeChildren(cur, mn, false)
	}
}

func (e *Editor) refreshTreeLabels() {
	root := e.tree.GetRoot()
	if root == nil {
		return
	}
	root.Walk(func(node, _ *tview.TreeNode) bool {
		if mn, ok := node.GetReference().(*structmodel.Node); ok {
			node.SetText(formatTreeNodeText(mn))
			applyTreeNodeColor(node, mn)
		}
		return true
	})
}

func (e *Editor) navigateToChildPath(targetPath string) {
	cur := e.tree.GetCurrentNode()
	if cur == nil {
		return
	}
	mn, ok := cur.GetReference().(*structmodel.Node)
	if !ok {
		return
	}
	e.populateTreeChildren(cur, mn, false)
	cur.SetExpanded(true)
	for _, ch := range cur.GetChildren() {
		if cm, ok := ch.GetReference().(*structmodel.Node); ok && cm.Path() == targetPath {
			e.tree.SetCurrentNode(ch)
			e.selectModelNode(cm, true)
			return
		}
	}
}

func (e *Editor) selectModelNode(mn *structmodel.Node, moveFocus bool) {
	e.currentNode = mn
	e.currentHelp = mn.Tag().Help
	e.currentErr = nil
	e.detail.SetTitle(fmt.Sprintf(" %s ", mn.Label()))

	e.renderDetail(mn)
	e.updateHeader()
	e.updateFooter()
	if moveFocus {
		e.focusDetail()
	}
}

func formatTreeNodeText(mn *structmodel.Node) string {
	sum := mn.Summary()
	if sum == "" {
		return tview.Escape(mn.Label())
	}
	return tview.Escape(fmt.Sprintf("%s %s", mn.Label(), sum))
}

func applyTreeNodeColor(tn *tview.TreeNode, mn *structmodel.Node) {
	switch {
	case mn.HasCycle():
		tn.SetColor(tcell.ColorRed)
	case mn.IsPointer() && mn.IsNil():
		tn.SetColor(tcell.ColorGray)
	case mn.TargetKind() == structmodel.KindSlice || mn.TargetKind() == structmodel.KindArray:
		tn.SetColor(tcell.ColorAqua)
	case mn.TargetKind() == structmodel.KindMap:
		tn.SetColor(tcell.ColorYellow)
	default:
		tn.SetColor(tcell.ColorWhite)
	}
}
