// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"cloudeng.io/cmdutil/cmdtypes"
	"cloudeng.io/cmdutil/flags"
	"cloudeng.io/tui/tview/structedit"
	"github.com/rivo/tview"
)

// OutputFormat implements flags.EnumType[OutputFormat].
type OutputFormat string

const (
	FormatText OutputFormat = "text"
	FormatJSON OutputFormat = "json"
	FormatYAML OutputFormat = "yaml"
)

func (OutputFormat) EnumValues() map[string]OutputFormat {
	return map[string]OutputFormat{
		"text": FormatText,
		"json": FormatJSON,
		"yaml": FormatYAML,
	}
}

// CmdTypesConfig exercises every configuration type in cloudeng.io/cmdutil/cmdtypes.
type CmdTypesConfig struct {
	ByteSize    cmdtypes.ByteSize    `yaml:"byte_size" doc:"Byte size (e.g. 16MiB, 10MB)"`
	FlexTime    cmdtypes.FlexTime    `yaml:"flex_time" doc:"Timestamp in RFC3339, DateTime, TimeOnly, or DateOnly"`
	Permissions cmdtypes.Permissions `yaml:"permissions" doc:"File mode (octal 0755, rwxr-xr-x, or u=rwx,go=rx)"`
}

// ScalarFlagsConfig exercises scalar and enum flag.Value types in cloudeng.io/cmdutil/flags.
type ScalarFlagsConfig struct {
	ByteSize    flags.ByteSize           `yaml:"byte_size" doc:"Flag byte size retaining unit notation (e.g. 32MiB)"`
	Time        flags.Time               `yaml:"time" doc:"Flag time (RFC3339, DateTime, DateOnly, relative -7d)"`
	Permissions flags.Permissions        `yaml:"permissions" doc:"Flag file permissions (e.g. u=rwx,go=rx or 0755)"`
	Format      flags.Enum[OutputFormat] `yaml:"format" doc:"Enumerated output format"`
	Mode        flags.OneOf              `yaml:"mode" doc:"Single choice validated with flags.OneOf" tview:"widget=dropdown,options=fast|balanced|thorough"`
}

// MultiFlagsConfig exercises multi-value slice flag types in cloudeng.io/cmdutil/flags.
type MultiFlagsConfig struct {
	Repeating flags.Repeating `yaml:"repeating" doc:"Values from repeated flag instances" tview:"expand"`
	Commas    flags.Commas    `yaml:"commas" doc:"Values from comma-separated flag instances" tview:"expand"`
}

// BoundsConfig exercises every Bound type in cloudeng.io/cmdutil/flags.
type BoundsConfig struct {
	Int      flags.IntBound      `yaml:"int_bound" doc:"Integer bound (e.g. 100 or +10)"`
	ByteSize flags.ByteSizeBound `yaml:"byte_size_bound" doc:"Byte size bound (e.g. 64MiB)"`
	Duration flags.DurationBound `yaml:"duration_bound" doc:"Duration bound (e.g. +1.5h, 30m)"`
	Time     flags.TimeBound     `yaml:"time_bound" doc:"Time bound (e.g. 2026-03-05, -7d)"`
	String   flags.StringBound   `yaml:"string_bound" doc:"String bound (e.g. m)"`
}

// IntervalsConfig exercises every Interval and Intervals type in cloudeng.io/cmdutil/flags.
type IntervalsConfig struct {
	Int          flags.IntInterval       `yaml:"int_interval" doc:"Integer interval (e.g. 1..100)"`
	ByteSize     flags.ByteSizeInterval  `yaml:"byte_size_interval" doc:"Byte size interval (e.g. 1KiB..1MiB)"`
	Duration     flags.DurationInterval  `yaml:"duration_interval" doc:"Duration interval (e.g. 1h..2d)"`
	Time         flags.TimeInterval      `yaml:"time_interval" doc:"Time interval (e.g. 2026-01-01..2026-12-31)"`
	String       flags.StringInterval    `yaml:"string_interval" doc:"String interval (e.g. a..m)"`
	IntList      flags.IntIntervals      `yaml:"int_intervals" doc:"Comma-separated integer intervals (e.g. 1..3,7,10..,^2)"`
	ByteSizeList flags.ByteSizeIntervals `yaml:"byte_size_intervals" doc:"Comma-separated byte size intervals (e.g. 1KB..4KB,8KB..16KB)"`
	DurationList flags.DurationIntervals `yaml:"duration_intervals" doc:"Comma-separated duration intervals (e.g. 1s..5s,10s..30s)"`
	TimeList     flags.TimeIntervals     `yaml:"time_intervals" doc:"Comma-separated time intervals (e.g. 2026-01-01..2026-01-15,2026-02-01..2026-02-15)"`
	StringList   flags.StringIntervals   `yaml:"string_intervals" doc:"Comma-separated string intervals (e.g. a..f,m..z)"`
}

// Config combines every cmdtypes and flags type into a single editable root struct.
type Config struct {
	CmdTypes    CmdTypesConfig    `yaml:"cmdtypes" doc:"Types from cloudeng.io/cmdutil/cmdtypes" tview:"expand"`
	ScalarFlags ScalarFlagsConfig `yaml:"scalar_flags" doc:"Scalar and enum flag types from cloudeng.io/cmdutil/flags" tview:"expand"`
	MultiFlags  MultiFlagsConfig  `yaml:"multi_flags" doc:"Repeating and Commas flag types from cloudeng.io/cmdutil/flags" tview:"expand"`
	Bounds      BoundsConfig      `yaml:"bounds" doc:"Bound types from cloudeng.io/cmdutil/flags"`
	Intervals   IntervalsConfig   `yaml:"intervals" doc:"Interval and Intervals types from cloudeng.io/cmdutil/flags"`
}

func (c Config) Validate() error {
	return c.ScalarFlags.Mode.Validate("fast", "balanced", "thorough")
}

func mustSet(setter interface{ Set(string) error }, v string) {
	if err := setter.Set(v); err != nil {
		panic(fmt.Sprintf("Set(%q): %v", v, err))
	}
}

func DefaultConfig() Config {
	var cfg Config
	cfg.CmdTypes = CmdTypesConfig{
		ByteSize:    16 * cmdtypes.MiB,
		FlexTime:    cmdtypes.FlexTime(time.Date(2026, 11, 17, 10, 30, 0, 0, time.UTC)),
		Permissions: 0o755,
	}

	cfg.ScalarFlags.Format = flags.Enum[OutputFormat]{Value: FormatJSON}
	cfg.ScalarFlags.Mode = "balanced"
	mustSet(&cfg.ScalarFlags.ByteSize, "32MiB")
	mustSet(&cfg.ScalarFlags.Time, "2026-11-17 09:00:00")
	mustSet(&cfg.ScalarFlags.Permissions, "u=rwx,go=rx")

	cfg.MultiFlags.Repeating = flags.Repeating{Values: []string{"alpha", "beta"}}
	cfg.MultiFlags.Commas = flags.Commas{Values: []string{"us-east", "eu-west"}}

	mustSet(&cfg.Bounds.Int, "100")
	mustSet(&cfg.Bounds.ByteSize, "64MiB")
	mustSet(&cfg.Bounds.Duration, "+1.5h")
	mustSet(&cfg.Bounds.Time, "2026-03-05")
	mustSet(&cfg.Bounds.String, "m")

	mustSet(&cfg.Intervals.Int, "1..100")
	mustSet(&cfg.Intervals.ByteSize, "1KiB..1MiB")
	mustSet(&cfg.Intervals.Duration, "1h..2d")
	mustSet(&cfg.Intervals.Time, "2026-01-01..2026-12-31")
	mustSet(&cfg.Intervals.String, "a..m")

	mustSet(&cfg.Intervals.IntList, "1..3,7,10..,^2")
	mustSet(&cfg.Intervals.ByteSizeList, "1KB..4KB,8KB..16KB")
	mustSet(&cfg.Intervals.DurationList, "1s..5s,10s..30s")
	mustSet(&cfg.Intervals.TimeList, "2026-01-01..2026-01-15,2026-02-01..2026-02-15")
	mustSet(&cfg.Intervals.StringList, "a..f,m..z")

	return cfg
}

func main() {
	cfg := DefaultConfig()
	app := tview.NewApplication()
	ed, err := structedit.New(
		&cfg,
		structedit.WithTitle("cmdutil Flags & Types"),
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
	fmt.Printf("Saved config: format=%s mode=%s size=%s\n",
		cfg.ScalarFlags.Format, cfg.ScalarFlags.Mode, cfg.CmdTypes.ByteSize)
}
