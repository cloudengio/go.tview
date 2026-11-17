// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package structmodel_test

import (
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"

	"cloudeng.io/tui/tview/structmodel"
)

type embeddedMeta struct {
	Owner string `yaml:"owner"`
	Env   string `yaml:"env" tview:"order=-5"`
}

type route struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
}

func (r *route) SetDefaults() {
	r.Name = "new-route"
	r.Port = 8080
}

func (r route) Validate() error {
	if r.Port <= 0 || r.Port > 65535 {
		return errors.New("port out of range")
	}
	return nil
}

type treeNode struct {
	Name     string      `yaml:"name"`
	Next     *treeNode   `yaml:"next"`
	Children []*treeNode `yaml:"children" tview:"itemlabel=Name"`
}

type sampleConfig struct {
	embeddedMeta `yaml:",inline"`
	Host         string           `yaml:"host" tview:"required"`
	Enabled      bool             `yaml:"enabled"`
	Timeout      time.Duration    `yaml:"timeout"`
	BindIP       netip.Addr       `yaml:"bind_ip"`
	Tags         []string         `yaml:"tags" tview:"widget=csv"`
	Ports        [2]int           `yaml:"ports"`
	Routes       []route          `yaml:"routes" tview:"itemlabel=Name"`
	Endpoints    map[string]route `yaml:"endpoints" tview:"itemlabel=Name"`
	Weights      map[string]int   `yaml:"weights"`
	Optional     *route           `yaml:"optional"`
	Tree         *treeNode        `yaml:"tree"`
	Secret       string           `yaml:"-" tview:"show,always-hide,label='API Secret'"`
	Hidden       string           `yaml:"hidden" tview:"-"`
}

func fieldMap(fields []*structmodel.FieldBinding) map[string]*structmodel.FieldBinding {
	m := make(map[string]*structmodel.FieldBinding, len(fields))
	for _, f := range fields {
		m[f.Tag().Name] = f
	}
	return m
}

func childMap(nodes []*structmodel.Node) map[string]*structmodel.Node {
	m := make(map[string]*structmodel.Node, len(nodes))
	for _, n := range nodes {
		m[n.Tag().Name] = n
	}
	return m
}

func mustSetString(t *testing.T, f *structmodel.FieldBinding, val string) {
	t.Helper()
	if err := f.SetString(val); err != nil {
		t.Fatalf("SetString(%s, %q): %v", f.Tag().Name, val, err)
	}
}

func TestDocumentInlineOrderAndVisibility(t *testing.T) {
	cfg := sampleConfig{
		embeddedMeta: embeddedMeta{Owner: "platform", Env: "dev"},
		Host:         "localhost",
		Secret:       "init",
	}
	doc, err := structmodel.NewDocument(&cfg)
	if err != nil {
		t.Fatalf("NewDocument: %v", err)
	}
	fields := doc.Root().ScalarFields()
	if len(fields) == 0 || fields[0].Tag().Name != "env" {
		t.Fatalf("expected 'env' sorted first via order=-5, got %v", fields)
	}
	byName := fieldMap(fields)
	if _, ok := byName["hidden"]; ok {
		t.Fatal("expected tview:\"-\" field to be hidden")
	}
	if _, ok := byName["Secret"]; !ok {
		t.Fatal("expected yaml:\"-\" tview:\"show\" field to be visible")
	}
}

func TestDocumentCommitAndRevert(t *testing.T) {
	cfg := sampleConfig{
		embeddedMeta: embeddedMeta{Owner: "platform", Env: "dev"},
		Host:         "localhost",
		Enabled:      true,
		Timeout:      5 * time.Second,
		BindIP:       netip.MustParseAddr("127.0.0.1"),
		Tags:         []string{"alpha", "beta"},
	}

	doc, err := structmodel.NewDocument(&cfg)
	if err != nil {
		t.Fatalf("NewDocument: %v", err)
	}
	byName := fieldMap(doc.Root().ScalarFields())

	mustSetString(t, byName["owner"], "core")
	mustSetString(t, byName["timeout"], "15s")
	mustSetString(t, byName["bind_ip"], "10.0.0.1")
	mustSetString(t, byName["tags"], "x, y, z")
	if err := byName["enabled"].SetBool(false); err != nil {
		t.Fatalf("SetBool enabled: %v", err)
	}
	if !doc.Dirty() || cfg.Owner != "platform" {
		t.Fatalf("expected dirty working copy without mutating original")
	}
	if err := doc.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if doc.Dirty() || cfg.Owner != "core" || cfg.Timeout != 15*time.Second {
		t.Fatalf("unexpected committed config: %+v", cfg)
	}
	if cfg.BindIP.String() != "10.0.0.1" || len(cfg.Tags) != 3 || cfg.Enabled {
		t.Fatalf("unexpected committed fields: %+v", cfg)
	}

	mustSetString(t, byName["host"], "changed.example")
	doc.Revert()
	if doc.Dirty() || doc.WorkingValue().(*sampleConfig).Host != "localhost" {
		t.Fatalf("unexpected state after Revert")
	}
}

func TestDocumentValidationBlocksCommit(t *testing.T) {
	cfg := sampleConfig{Host: "localhost", Timeout: 5 * time.Second}
	doc, err := structmodel.NewDocument(&cfg)
	if err != nil {
		t.Fatalf("NewDocument: %v", err)
	}
	byName := fieldMap(doc.Root().ScalarFields())
	if err := byName["timeout"].SetString("not-a-duration"); err == nil {
		t.Fatal("expected duration parse error")
	}
	if err := doc.Commit(); err == nil {
		t.Fatal("expected Commit to fail with pending field error")
	}
}

func TestSlicesAndFixedArrays(t *testing.T) {
	cfg := sampleConfig{
		Host: "localhost",
		Routes: []route{
			{Name: "api", Port: 8080},
			{Name: "admin", Port: 9090},
		},
	}
	doc, err := structmodel.NewDocument(&cfg)
	if err != nil {
		t.Fatalf("NewDocument: %v", err)
	}
	children := childMap(doc.Root().Children())

	routesNode := children["routes"]
	idx, err := routesNode.AppendItem()
	if err != nil || idx != 2 {
		t.Fatalf("AppendItem: idx=%d err=%v", idx, err)
	}
	if routesNode.ItemSummary(2) != "new-route" {
		t.Fatalf("expected SetDefaults itemlabel 'new-route', got %q", routesNode.ItemSummary(2))
	}
	if err := routesNode.MoveItem(2, 0); err != nil {
		t.Fatalf("MoveItem: %v", err)
	}
	if _, err := routesNode.DuplicateItem(0); err != nil {
		t.Fatalf("DuplicateItem: %v", err)
	}
	if err := routesNode.DeleteItem(3); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}
	if routesNode.Len() != 3 {
		t.Fatalf("expected 3 routes, got %d", routesNode.Len())
	}

	portsNode := children["ports"]
	if !portsNode.IsFixedArray() {
		t.Fatal("expected IsFixedArray=true")
	}
	if _, err := portsNode.AppendItem(); err == nil {
		t.Fatal("expected AppendItem error on fixed array")
	}
	if err := portsNode.SetItemScalar(1, "443"); err != nil {
		t.Fatalf("SetItemScalar: %v", err)
	}
	if err := doc.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if cfg.Ports[1] != 443 || len(cfg.Routes) != 3 {
		t.Fatalf("unexpected committed slices/arrays: %+v", cfg)
	}
}

func TestMaps(t *testing.T) {
	cfg := sampleConfig{
		Host:      "localhost",
		Endpoints: map[string]route{"prod": {Name: "prod-api", Port: 443}},
		Weights:   map[string]int{"a": 1, "b": 2},
	}
	doc, err := structmodel.NewDocument(&cfg)
	if err != nil {
		t.Fatalf("NewDocument: %v", err)
	}
	children := childMap(doc.Root().Children())

	epNode := children["endpoints"]
	if err := epNode.AddMapEntry("staging"); err != nil {
		t.Fatalf("AddMapEntry: %v", err)
	}
	if err := epNode.RenameMapKey("staging", "stage"); err != nil {
		t.Fatalf("RenameMapKey: %v", err)
	}
	if keys := epNode.MapKeys(); len(keys) != 2 || keys[0] != "prod" || keys[1] != "stage" {
		t.Fatalf("unexpected sorted keys: %v", keys)
	}
	for _, ch := range epNode.Children() {
		if ch.Label() == "stage" {
			mustSetString(t, fieldMap(ch.ScalarFields())["port"], "8443")
		}
	}

	wNode := children["weights"]
	if err := wNode.SetMapEntryScalar("c", "42"); err != nil {
		t.Fatalf("SetMapEntryScalar: %v", err)
	}
	if err := wNode.DeleteMapEntry("a"); err != nil {
		t.Fatalf("DeleteMapEntry: %v", err)
	}
	if err := doc.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if cfg.Endpoints["stage"].Port != 8443 || cfg.Weights["c"] != 42 {
		t.Fatalf("unexpected committed map fields: %+v", cfg)
	}
	if _, exists := cfg.Weights["a"]; exists {
		t.Fatal("expected deleted map key 'a' to be absent")
	}
}

func TestPointersInitAndClear(t *testing.T) {
	cfg := sampleConfig{Host: "localhost"}
	doc, err := structmodel.NewDocument(&cfg)
	if err != nil {
		t.Fatalf("NewDocument: %v", err)
	}
	optNode := childMap(doc.Root().Children())["optional"]
	if !optNode.IsPointer() || !optNode.IsNil() {
		t.Fatal("expected nil optional pointer")
	}
	if err := optNode.InitPtr(); err != nil {
		t.Fatalf("InitPtr: %v", err)
	}
	if err := doc.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if cfg.Optional == nil || cfg.Optional.Port != 8080 {
		t.Fatalf("expected initialized Optional, got %+v", cfg.Optional)
	}
	if err := optNode.ClearPtr(); err != nil {
		t.Fatalf("ClearPtr: %v", err)
	}
	if err := doc.Commit(); err != nil {
		t.Fatalf("Commit after ClearPtr: %v", err)
	}
	if cfg.Optional != nil {
		t.Fatalf("expected nil Optional after ClearPtr, got %+v", cfg.Optional)
	}
}

func TestRecursiveStructsAndCycleGuard(t *testing.T) {
	n1 := &treeNode{Name: "root-node"}
	n2 := &treeNode{Name: "child-node"}
	n1.Next = n2
	n2.Next = n1
	cfg := sampleConfig{Host: "localhost", Tree: n1}

	doc, err := structmodel.NewDocument(&cfg)
	if err != nil {
		t.Fatalf("NewDocument recursive: %v", err)
	}
	tree := childMap(doc.Root().Children())["tree"]
	if tree == nil {
		t.Fatal("missing tree node")
	}
	next1 := childMap(tree.Children())["next"]
	if next1 == nil || next1.HasCycle() {
		t.Fatalf("expected non-cyclic first hop, got %+v", next1)
	}
	next2 := childMap(next1.Children())["next"]
	if next2 == nil || !next2.HasCycle() || len(next2.Children()) != 0 {
		t.Fatalf("expected cycle detection at second hop, got %+v", next2)
	}
}

type badCyclicInline struct {
	Self *badCyclicInline `yaml:",inline"`
}

type customInt int

type customCodec struct{}

func (customCodec) Format(v reflect.Value) (string, error) {
	return "custom:" + strings.Repeat("x", int(v.Int())), nil
}

func (customCodec) Parse(t reflect.Type, text string) (reflect.Value, error) {
	out := reflect.New(t).Elem()
	out.SetInt(int64(len(strings.TrimPrefix(text, "custom:"))))
	return out, nil
}

func TestCyclicInlineErrorAndCustomCodec(t *testing.T) {
	if _, err := structmodel.BuildSchema(reflect.TypeFor[badCyclicInline](), nil); err == nil {
		t.Fatal("expected error for cyclic inline struct")
	}

	type customCfg struct {
		Val customInt `yaml:"val"`
	}
	c := customCfg{Val: 3}
	doc, err := structmodel.NewDocument(&c, structmodel.WithCodec(reflect.TypeFor[customInt](), customCodec{}))
	if err != nil {
		t.Fatalf("NewDocument custom codec: %v", err)
	}
	f := doc.Root().ScalarFields()[0]
	if f.String() != "custom:xxx" {
		t.Fatalf("unexpected formatted custom value: %q", f.String())
	}
	if err := f.SetString("custom:xxxxx"); err != nil {
		t.Fatalf("SetString custom: %v", err)
	}
	if err := doc.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if c.Val != 5 {
		t.Fatalf("expected Val=5, got %d", c.Val)
	}
}

type secretFormat string

const (
	secretRaw    secretFormat = "raw"
	secretHex    secretFormat = "hex"
	secretBase64 secretFormat = "base64"
)

func (secretFormat) EnumValues() map[string]secretFormat {
	return map[string]secretFormat{
		"raw":    secretRaw,
		"hex":    secretHex,
		"base64": secretBase64,
	}
}

type enumWrapper[T interface {
	comparable
	EnumValues() map[string]T
}] struct {
	Value T
}

func (e enumWrapper[T]) MarshalText() ([]byte, error) {
	for k, v := range e.Value.EnumValues() {
		if v == e.Value {
			return []byte(k), nil
		}
	}
	return nil, errors.New("unknown enum value")
}

func (e *enumWrapper[T]) UnmarshalText(text []byte) error {
	v, ok := e.Value.EnumValues()[string(text)]
	if !ok {
		return errors.New("invalid enum value")
	}
	e.Value = v
	return nil
}

func (e enumWrapper[T]) AllowedValues() string {
	return "base64, hex, raw"
}

type keychainKind int

const (
	keychainFile keychainKind = iota + 1
	keychainICloud
	keychainAll
)

func (keychainKind) EnumValues() map[string]keychainKind {
	return map[string]keychainKind{
		"file":   keychainFile,
		"icloud": keychainICloud,
		"all":    keychainAll,
	}
}

type customProvider string

func (customProvider) EnumOptions() []string { return []string{"z", "a", "m"} }

func TestEnumExtractionAndCodecs(t *testing.T) {
	type enumCfg struct {
		Wrapped  enumWrapper[secretFormat] `yaml:"wrapped"`
		Direct   keychainKind              `yaml:"direct"`
		Provider customProvider            `yaml:"provider"`
		Override keychainKind              `yaml:"override" tview:"options=file|icloud"`
	}

	cfg := enumCfg{
		Wrapped:  enumWrapper[secretFormat]{Value: secretHex},
		Direct:   keychainICloud,
		Provider: "a",
		Override: keychainFile,
	}

	doc, err := structmodel.NewDocument(&cfg)
	if err != nil {
		t.Fatalf("NewDocument: %v", err)
	}
	byName := fieldMap(doc.Root().ScalarFields())

	if got := byName["wrapped"].Tag().Options; !reflect.DeepEqual(got, []string{"base64", "hex", "raw"}) {
		t.Fatalf("wrapped options: got %v", got)
	}
	if got := byName["direct"].Tag().Options; !reflect.DeepEqual(got, []string{"all", "file", "icloud"}) {
		t.Fatalf("direct options: got %v", got)
	}
	if got := byName["provider"].Tag().Options; !reflect.DeepEqual(got, []string{"a", "m", "z"}) {
		t.Fatalf("provider options: got %v", got)
	}
	if got := byName["override"].Tag().Options; !reflect.DeepEqual(got, []string{"file", "icloud"}) {
		t.Fatalf("override options should take precedence, got %v", got)
	}

	mustSetString(t, byName["wrapped"], "base64")
	if err := byName["direct"].SetString("invalid"); err == nil {
		t.Fatal("expected error for invalid direct enum value")
	}
	if err := doc.Commit(); err == nil {
		t.Fatal("expected Commit to fail while enum error is pending")
	}
	mustSetString(t, byName["direct"], "all")
	if err := doc.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if cfg.Wrapped.Value != secretBase64 || cfg.Direct != keychainAll {
		t.Fatalf("unexpected committed enums: %+v", cfg)
	}
}
