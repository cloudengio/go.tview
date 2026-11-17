// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"

	"cloudeng.io/tui/tview/structedit"
	"cloudeng.io/tui/tview/structmodel"
	basepool "cloudeng.io/vms/vmspool"
	"github.com/rivo/tview"
)

func editorOptions(app *tview.Application) []structedit.Option {
	return []structedit.Option{
		structedit.WithTitle("GitHub Runner Orchestrator"),
		structedit.WithApp(app),
		structedit.WithEnumOptions(reflect.TypeFor[basepool.StagingBehaviour](), []string{
			"Running",
			"Suspended",
			"Stopped",
		}),
	}
}

func main() {
	overridesFile := flag.String("overrides", "", "JSON file for saving/applying setting overrides over the parsed YAML config")
	settingsFile := flag.String("settings", "", "JSON file for saving the full settings snapshot")
	flag.Parse()

	cfg := DefaultConfig()
	if *overridesFile != "" {
		if err := structmodel.ApplyJSONFile(&cfg, *overridesFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Fatalf("apply overrides %s: %v", *overridesFile, err)
		}
	}

	app := tview.NewApplication()
	opts := editorOptions(app)
	if *overridesFile != "" {
		opts = append(opts, structedit.WithOverridesFile(*overridesFile))
	}
	if *settingsFile != "" {
		opts = append(opts, structedit.WithSettingsFile(*settingsFile))
	}
	opts = append(opts,
		structedit.WithOnSave(func(_ any) error {
			app.Stop()
			return nil
		}),
		structedit.WithOnCancel(func() {
			app.Stop()
		}),
	)
	ed, err := structedit.New(&cfg, opts...)
	if err != nil {
		log.Fatal(err)
	}
	if err := app.SetRoot(ed, true).EnableMouse(true).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Saved %d repositories and %d VM pools\n", len(cfg.Repositories), len(cfg.VMPools))
}
