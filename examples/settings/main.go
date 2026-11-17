// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"net/netip"
	"os"
	"time"

	"cloudeng.io/tui/tview/structedit"
	"github.com/rivo/tview"
)

type metadata struct {
	Owner       string `yaml:"owner" tview:"order=-10,help='Owning team or service'"`
	Environment string `yaml:"env" tview:"order=-9,widget=dropdown,options=dev|staging|prod"`
}

type tlsConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file" tview:"placeholder='/etc/ssl/cert.pem'"`
	KeyFile  string `yaml:"key_file" tview:"placeholder='/etc/ssl/key.pem'"`
}

type route struct {
	Name     string        `yaml:"name"`
	Upstream string        `yaml:"upstream"`
	Timeout  time.Duration `yaml:"timeout"`
}

func (r *route) SetDefaults() {
	r.Name = "new-route"
	r.Upstream = "http://127.0.0.1:8080"
	r.Timeout = 5 * time.Second
}

type menuNode struct {
	Title    string      `yaml:"title"`
	Target   string      `yaml:"target"`
	Next     *menuNode   `yaml:"next"`
	Children []*menuNode `yaml:"children" tview:"itemlabel=Title"`
}

type appConfig struct {
	metadata     `yaml:",inline"`
	Listen       netip.Addr       `yaml:"listen" tview:"help='Bind IP address'"`
	Port         int              `yaml:"port"`
	LogLevel     string           `yaml:"log_level" tview:"widget=dropdown,options=debug|info|warn|error"`
	APIKey       string           `yaml:"-" tview:"show,hide-with-show,label='API Key',help='Overrides yaml:\"-\" with toggleable masking'"`
	SigningToken string           `yaml:"-" tview:"show,always-hide,label='Signing Token',help='Always shown as a fixed number of asterisks'"`
	Tags         []string         `yaml:"tags" tview:"widget=csv,help='Comma-separated labels'"`
	TLS          *tlsConfig       `yaml:"tls" tview:"expand"`
	Routes       []route          `yaml:"routes" tview:"itemlabel=Name"`
	Backends     map[string]route `yaml:"backends" tview:"itemlabel=Name"`
	Menu         *menuNode        `yaml:"menu"`
}

func main() {
	cfg := appConfig{
		metadata: metadata{
			Owner:       "platform",
			Environment: "dev",
		},
		Listen:       netip.MustParseAddr("127.0.0.1"),
		Port:         8080,
		LogLevel:     "info",
		APIKey:       "demo-secret",
		SigningToken: "very-long-signing-secret",
		Tags:         []string{"edge", "v1"},
		Routes: []route{
			{Name: "api", Upstream: "http://127.0.0.1:9000", Timeout: 10 * time.Second},
		},
		Backends: map[string]route{
			"primary": {Name: "main", Upstream: "http://10.0.0.5:8080", Timeout: 3 * time.Second},
		},
		Menu: &menuNode{
			Title:  "Root",
			Target: "/",
			Children: []*menuNode{
				{Title: "Status", Target: "/status"},
			},
		},
	}

	app := tview.NewApplication()
	ed, err := structedit.New(
		&cfg,
		structedit.WithTitle("AppConfig"),
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
	fmt.Printf("Final config: %+v\n", cfg)
}
