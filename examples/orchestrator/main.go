// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"os"

	"cloudeng.io/tui/tview/structedit"
	"github.com/rivo/tview"
)

func main() {
	cfg := DefaultConfig()
	app := tview.NewApplication()
	ed, err := structedit.New(
		&cfg,
		structedit.WithTitle("GitHub Runner Orchestrator"),
		structedit.WithApp(app),
		structedit.WithOnSave(func(_ any) error {
			app.Stop()
			return nil
		}),
		structedit.WithOnCancel(func() {
			app.Stop()
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	if err := app.SetRoot(ed, true).EnableMouse(true).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Saved %d repositories and %d VM pools\n", len(cfg.Repositories), len(cfg.VMPools))
}
