# Package [cloudeng.io/tui/tview/structedit](https://pkg.go.dev/cloudeng.io/tui/tview/structedit?tab=doc)

```go
import cloudeng.io/tui/tview/structedit
```

Package structedit provides a split-view rivo/tview settings editor for
arbitrary Go structs annotated with yaml and tview struct tags.

## Types
### Type Editor
```go
type Editor struct {
	*tview.Pages
	// contains filtered or unexported fields
}
```
Editor is a split-view tview.Primitive for editing an arbitrary Go struct
via yaml and tview struct tags.

### Functions

```go
func New(target any, opts ...Option) (*Editor, error)
```
New constructs a split-view Editor bound to target, which must be a non-nil
pointer to a struct.



### Methods

```go
func (e *Editor) Cancel()
```
Cancel requests cancellation. If unsaved edits exist, a confirmation modal
is shown unless read-only.


```go
func (e *Editor) Document() *structmodel.Document
```
Document returns the underlying structmodel.Document.


```go
func (e *Editor) Draw(screen tcell.Screen)
```
Draw synchronizes the description panel with the currently focused form
field before drawing the editor.


```go
func (e *Editor) Focus(delegate func(p tview.Primitive))
```
Focus delegates initial focus to the tree pane (or active modal) and
captures the container's focus setter.


```go
func (e *Editor) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive))
```
InputHandler handles editor-wide shortcuts before delegating to child panes.


```go
func (e *Editor) Revert()
```
Revert discards staged edits and refreshes the entire editor UI.


```go
func (e *Editor) Save() error
```
Save validates and commits all staged changes back into the target struct,
writes any configured settings/overrides JSON files, and invokes WithOnSave.




### Type Option
```go
type Option func(*editorConfig)
```
Option configures an Editor.

### Functions

```go
func WithApp(app *tview.Application) Option
```
WithApp attaches a tview.Application for explicit focus updates when
switching panes or dismissing modals.


```go
func WithCodec(t reflect.Type, codec structmodel.Codec) Option
```
WithCodec registers a custom scalar Codec for type t.


```go
func WithEnumOptions(t reflect.Type, options []string) Option
```
WithEnumOptions registers explicit dropdown choices for an external enum or
scalar type t that does not implement EnumValues or EnumOptionProvider.


```go
func WithOnCancel(fn func()) Option
```
WithOnCancel sets a callback invoked when the user cancels editing.


```go
func WithOnChange(fn func()) Option
```
WithOnChange sets a callback invoked whenever a field or collection is
edited.


```go
func WithOnSave(fn func(target any) error) Option
```
WithOnSave sets a callback invoked after Document.Commit succeeds.


```go
func WithOverridesFile(path string) Option
```
WithOverridesFile saves only modified fields as a JSON merge patch to path
(merging into any existing overrides at path) when Save succeeds.


```go
func WithReadOnly(readOnly bool) Option
```
WithReadOnly disables all edits across the editor.


```go
func WithSettingsFile(path string) Option
```
WithSettingsFile writes the full edited settings as JSON to path when Save
succeeds, leaving any source YAML file and its comments untouched.


```go
func WithTitle(title string) Option
```
WithTitle sets the title shown in the editor header bar.


```go
func WithTypeWidget(t reflect.Type, factory WidgetFactory) Option
```
WithTypeWidget registers a custom WidgetFactory for fields of type t.


```go
func WithValidator(fn func(target any) error) Option
```
WithValidator registers a document-level validation function checked on
Save.


```go
func WithWidget(name string, factory WidgetFactory) Option
```
WithWidget registers a custom WidgetFactory invoked when a field specifies
tview:"widget=<name>".




### Type WidgetFactory
```go
type WidgetFactory func(field *structmodel.FieldBinding, notifyChange func()) tview.FormItem
```
WidgetFactory constructs a custom tview.FormItem bound to field. Callers
should invoke notifyChange whenever the widget mutates field.





