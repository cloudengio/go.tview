# Package [cloudeng.io/tui/tview/structtag](https://pkg.go.dev/cloudeng.io/tui/tview/structtag?tab=doc)

```go
import cloudeng.io/tui/tview/structtag
```

Package structtag parses yaml and tview struct tags into unified field
metadata for UI generation.

## Types
### Type MaskMode
```go
type MaskMode int
```
MaskMode controls whether and how a field's value is masked in the UI.

### Constants
### MaskNone, MaskAlwaysHide, MaskHideWithShow
```go
const (
	// MaskNone displays the field value normally.
	MaskNone MaskMode = iota
	// MaskAlwaysHide always displays a fixed number of asterisks regardless of
	// value length and provides no reveal toggle.
	MaskAlwaysHide
	// MaskHideWithShow displays asterisks by default with an option to reveal
	// the underlying value.
	MaskHideWithShow
)
```




### Type Spec
```go
type Spec struct {
	// FieldName is the Go struct field name.
	FieldName string
	// YAMLName is the field name derived from the yaml tag (empty if absent or "-").
	YAMLName string
	// Name is the effective serialized key name (YAMLName if set, otherwise FieldName).
	Name string
	// Label is the display label shown in the UI (tview label if set, otherwise Name).
	Label string
	// Hidden indicates whether the field should be omitted from the UI.
	Hidden bool
	// Inline indicates that nested struct fields should be flattened into the parent.
	Inline bool
	// Widget overrides the default widget selection (e.g. "input", "textarea",
	// "checkbox", "dropdown", "csv", "lines", or a custom name).
	Widget string
	// Options lists allowed choices for dropdown/enum widgets.
	Options []string
	// Help provides contextual documentation for the field.
	Help string
	// Placeholder provides input placeholder text.
	Placeholder string
	// ItemLabel names a child field used to summarize slice elements.
	ItemLabel string
	// Order controls relative field ordering (smaller values appear first).
	Order int
	// ReadOnly prevents editing the field in the UI.
	ReadOnly bool
	// Mask controls masking of sensitive field values.
	Mask MaskMode
	// Required marks the field as mandatory.
	Required bool
	// Expand hints that tree nodes for this field should start expanded.
	Expand bool
}
```
Spec describes the resolved metadata for a struct field after merging its
yaml, help/doc, and tview struct tags.

### Functions

```go
func Parse(field reflect.StructField) (Spec, error)
```
Parse inspects field's yaml, doc/help, and tview tags and returns the merged
Spec. tview tag directives override yaml tag defaults.







