# Package [cloudeng.io/tui/tview/structmodel](https://pkg.go.dev/cloudeng.io/tui/tview/structmodel?tab=doc)

```go
import cloudeng.io/tui/tview/structmodel
```

Package structmodel provides reflection-based schema construction, value
bindings, scalar codecs, and lazy tree navigation for arbitrary Go structs
annotated with yaml and tview struct tags.

## Functions
### Func ApplyJSON
```go
func ApplyJSON(target any, data []byte, opts ...ApplyOption) error
```
ApplyJSON applies JSON-encoded settings (either a full snapshot or a partial
RFC 7396 merge patch) onto target, which must be a non-nil pointer to a
struct. Fields omitted from JSON are preserved untouched on target.

### Func ApplyJSONFile
```go
func ApplyJSONFile(target any, path string, opts ...ApplyOption) error
```
ApplyJSONFile reads a JSON settings file from path and applies it to target.

### Func DeepCopy
```go
func DeepCopy(src reflect.Value) reflect.Value
```
DeepCopy returns a deep clone of src, guarding against pointer cycles.

### Func ExtractEnumOptions
```go
func ExtractEnumOptions(t reflect.Type) ([]string, bool)
```
ExtractEnumOptions inspects t for enum value metadata and returns the sorted
list of allowed option strings if t represents an enum.

It recognizes:
  - types implementing EnumOptionProvider (EnumOptions() []string)
  - types implementing EnumValues() map[string]T (matching
    cmdutil/flags.EnumType[T])
  - wrapper structs with a Value field whose type implements EnumValues()
    map[string]T or with an AllowedValues() string method (matching
    cmdutil/flags.Enum[T])

### Func MarshalJSON
```go
func MarshalJSON(target any, opts ...Option) ([]byte, error)
```
MarshalJSON encodes target (a struct or pointer to struct) into indented
JSON using the same yaml/tview field names, inline flattening, visibility
rules, and scalar codecs used by the editor.

### Func MarshalOverridesJSON
```go
func MarshalOverridesJSON(base, updated any, opts ...Option) ([]byte, error)
```
MarshalOverridesJSON encodes only the fields and entries in updated that
differ from base as an RFC 7396 JSON merge patch. If nothing differs,
it returns "{}\n".

### Func MergeJSON
```go
func MergeJSON(baseJSON, patchJSON []byte) ([]byte, error)
```
MergeJSON applies an RFC 7396 JSON merge patch onto baseJSON.

### Func SaveJSONFile
```go
func SaveJSONFile(path string, target any, opts ...Option) error
```
SaveJSONFile encodes target with MarshalJSON and writes it atomically to
path.

### Func SaveOverridesJSONFile
```go
func SaveOverridesJSONFile(path string, base, updated any, opts ...Option) error
```
SaveOverridesJSONFile computes MarshalOverridesJSON(base, updated), merges
it onto any existing JSON file at path (if present), and writes the result.



## Types
### Type ApplyOption
```go
type ApplyOption func(*applyConfig)
```
ApplyOption configures how ApplyJSON and ApplyJSONFile apply JSON settings
onto a target struct.

### Functions

```go
func WithApplyOptions(opts ...Option) ApplyOption
```
WithApplyOptions passes structmodel.Option values (such as WithCodec or
WithEnumOptions) when building the schema for ApplyJSON.


```go
func WithReplaceMaps(replace bool) ApplyOption
```
WithReplaceMaps replaces map contents completely with the JSON object rather
than merging keys (where null deletes a key).


```go
func WithStrictJSON(strict bool) ApplyOption
```
WithStrictJSON causes ApplyJSON to fail if the JSON contains keys not
present in the target struct schema.




### Type Codec
```go
type Codec interface {
	Format(v reflect.Value) (string, error)
	Parse(t reflect.Type, text string) (reflect.Value, error)
}
```
Codec formats and parses values of a scalar Go type.


### Type CodecRegistry
```go
type CodecRegistry struct {
	// contains filtered or unexported fields
}
```
CodecRegistry maps specific Go reflect.Types to custom Codecs and handles
built-in primitive and encoding.TextMarshaler/TextUnmarshaler types.

### Functions

```go
func NewCodecRegistry() *CodecRegistry
```
NewCodecRegistry returns an empty CodecRegistry.



### Methods

```go
func (r *CodecRegistry) Classify(t reflect.Type) (ScalarKind, bool)
```
Classify returns whether t can be edited as a scalar and its ScalarKind.


```go
func (r *CodecRegistry) Format(v reflect.Value) (string, error)
```
Format converts v into its string representation.


```go
func (r *CodecRegistry) Parse(t reflect.Type, text string) (reflect.Value, error)
```
Parse converts text into a reflect.Value assignable to t.


```go
func (r *CodecRegistry) Register(t reflect.Type, c Codec)
```
Register associates a custom Codec with t.




### Type DefaultInitializer
```go
type DefaultInitializer interface {
	SetDefaults()
}
```
DefaultInitializer may be implemented by structs to populate default values
when a nil pointer is initialized or a new slice/map item is created.


### Type Document
```go
type Document struct {
	// contains filtered or unexported fields
}
```
Document manages a transactional editing session for a target struct
pointer. Edits mutate an isolated working copy until Commit is called.

### Functions

```go
func NewDocument(target any, opts ...Option) (*Document, error)
```
NewDocument creates a Document bound to target, which must be a non-nil
pointer to a struct.



### Methods

```go
func (d *Document) Commit() error
```
Commit validates the working copy and copies it into the original target
struct if valid.


```go
func (d *Document) Dirty() bool
```
Dirty reports whether any edits have been made since creation or the last
Commit/Revert.


```go
func (d *Document) OverridesJSON() ([]byte, error)
```
OverridesJSON encodes only the changes made in this Document relative to the
original target struct.


```go
func (d *Document) Revert()
```
Revert discards all staged edits and restores the working copy from the
original target struct.


```go
func (d *Document) Root() *Node
```
Root returns the root Node of the lazy configuration tree.


```go
func (d *Document) Schema() *Schema
```
Schema returns the compiled Schema for the document's root type.


```go
func (d *Document) Validate() error
```
Validate checks pending field parse/required errors, runs Validator.Validate
on any structs in the working tree that implement it, and runs any custom
document validator.


```go
func (d *Document) WorkingValue() any
```
WorkingValue returns the working copy pointer passed to validators or
callers inspecting staged edits.




### Type EnumOptionProvider
```go
type EnumOptionProvider interface {
	EnumOptions() []string
}
```
EnumOptionProvider may be implemented by enum types to supply their allowed
string representations directly.


### Type FieldBinding
```go
type FieldBinding struct {
	// contains filtered or unexported fields
}
```
FieldBinding connects a scalar (or inline string slice) form field to its
underlying ValueRef in the Document.

### Methods

```go
func (f *FieldBinding) Bool() bool
```
Bool returns the boolean value for ScalarBool fields.


```go
func (f *FieldBinding) Error() error
```
Error returns the last parse or validation error recorded for this field.


```go
func (f *FieldBinding) ID() string
```
ID returns the unique path identifier for this field.


```go
func (f *FieldBinding) IsSliceString() bool
```
IsSliceString reports whether this field binds a []string via csv or lines.


```go
func (f *FieldBinding) ReadOnly() bool
```
ReadOnly reports whether editing this field is disabled.


```go
func (f *FieldBinding) ScalarKind() ScalarKind
```
ScalarKind returns the ScalarKind of this field.


```go
func (f *FieldBinding) SetBool(b bool) error
```
SetBool updates a ScalarBool field.


```go
func (f *FieldBinding) SetString(text string) error
```
SetString parses text and assigns the value if valid, recording any parse or
required-field error on the Document so Commit will reject invalid inputs.


```go
func (f *FieldBinding) String() string
```
String formats the current value as text for display in form inputs.


```go
func (f *FieldBinding) Tag() structtag.Spec
```
Tag returns the merged structtag.Spec for this field.


```go
func (f *FieldBinding) Type() reflect.Type
```
Type returns the Go reflect.Type of this field.




### Type FieldDesc
```go
type FieldDesc struct {
	Tag               structtag.Spec
	Path              []int
	Type              *TypeDesc
	DeclIndex         int
	InlineStringSlice bool
}
```
FieldDesc describes a single visible field on a KindStruct TypeDesc.


### Type Kind
```go
type Kind int
```
Kind identifies the structural category of a TypeDesc.

### Constants
### KindScalar, KindStruct, KindSlice, KindArray, KindMap, KindPtr
```go
const (
	KindScalar Kind = iota + 1
	KindStruct
	KindSlice
	KindArray
	KindMap
	KindPtr
)
```




### Type Node
```go
type Node struct {
	// contains filtered or unexported fields
}
```
Node represents an addressable structural item (struct, pointer, slice,
array, or map) in the lazy configuration tree.

### Methods

```go
func (n *Node) AddMapEntry(key string) error
```
AddMapEntry inserts a new entry under key with a default-initialized value.


```go
func (n *Node) AppendItem() (int, error)
```
AppendItem appends a new default-initialized element to a slice node and
returns its index.


```go
func (n *Node) ChildAt(i int) *Node
```
ChildAt returns the structural child Node at index i of a slice or array
node, or nil if out of range or elements are scalars.


```go
func (n *Node) Children() []*Node
```
Children lazily returns structural child nodes (nested structs, pointers,
slices, arrays, and maps) for tree navigation.


```go
func (n *Node) ClearPtr() error
```
ClearPtr resets a pointer node to nil and clears any nested field errors.


```go
func (n *Node) DeleteItem(i int) error
```
DeleteItem removes element i from a slice node.


```go
func (n *Node) DeleteMapEntry(key string) error
```
DeleteMapEntry removes key from a map node.


```go
func (n *Node) DuplicateItem(i int) (int, error)
```
DuplicateItem clones element i and appends it to a slice node.


```go
func (n *Node) ElemEnumOptions() []string
```
ElemEnumOptions returns any extracted enum options for slice/array/map
elements.


```go
func (n *Node) ElemIsScalar() bool
```
ElemIsScalar reports whether slice/array/map elements are scalars.


```go
func (n *Node) HasCycle() bool
```
HasCycle reports whether this node points to an already-visited ancestor
pointer in memory, preventing infinite tree expansion.


```go
func (n *Node) InitPtr() error
```
InitPtr allocates a zero-initialized value (calling DefaultInitializer if
implemented) when this node is a nil pointer.


```go
func (n *Node) IsFixedArray() bool
```
IsFixedArray reports whether this node is a fixed-length Go array.


```go
func (n *Node) IsNil() bool
```
IsNil reports whether the underlying pointer, slice, or map is nil.


```go
func (n *Node) IsPointer() bool
```
IsPointer reports whether the underlying Go type is a pointer.


```go
func (n *Node) ItemScalar(i int) (string, error)
```
ItemScalar formats scalar element i of a slice or array.


```go
func (n *Node) ItemSummary(i int) string
```
ItemSummary returns a short summary for element i of a slice or array.


```go
func (n *Node) Label() string
```
Label returns the display label for this node.


```go
func (n *Node) Len() int
```
Len returns the number of elements in a slice, array, or map node.


```go
func (n *Node) MapChild(key string) *Node
```
MapChild returns the structural child Node for key of a map node, or nil if
absent or values are scalars.


```go
func (n *Node) MapEntrySummary(key string) string
```
MapEntrySummary returns a concise summary for the map value at key.


```go
func (n *Node) MapKeys() []string
```
MapKeys returns sorted formatted keys of a map node.


```go
func (n *Node) MoveItem(from, to int) error
```
MoveItem swaps or shifts an element from index from to index to in a slice
or array.


```go
func (n *Node) Path() string
```
Path returns the dot/bracket path to this node.


```go
func (n *Node) ReadOnly() bool
```
ReadOnly reports whether edits on this node are disabled.


```go
func (n *Node) RenameMapKey(oldKey, newKey string) error
```
RenameMapKey renames an existing map entry from oldKey to newKey.


```go
func (n *Node) ScalarFields() []*FieldBinding
```
ScalarFields returns the scalar (and inline-string-slice) bindings editable
directly in a form for this node.


```go
func (n *Node) SetItemScalar(i int, text string) error
```
SetItemScalar parses text and stores it at element i of a scalar
slice/array.


```go
func (n *Node) SetMapEntryScalar(key, text string) error
```
SetMapEntryScalar parses text and assigns it to a scalar map entry.


```go
func (n *Node) Summary() string
```
Summary returns a concise status suffix for display in trees and tables.


```go
func (n *Node) Tag() structtag.Spec
```
Tag returns the merged structtag.Spec for the field represented by this
node.


```go
func (n *Node) TargetKind() Kind
```
TargetKind returns the underlying non-pointer Kind of this node.


```go
func (n *Node) TargetTypeName() string
```
TargetTypeName returns a human-readable type name for the target value.




### Type Option
```go
type Option func(*docConfig)
```
Option configures a Document.

### Functions

```go
func WithCodec(t reflect.Type, c Codec) Option
```
WithCodec registers a custom scalar Codec for type t.


```go
func WithCodecs(reg *CodecRegistry) Option
```
WithCodecs uses an existing CodecRegistry.


```go
func WithEnumOptions(t reflect.Type, options []string) Option
```
WithEnumOptions registers explicit dropdown options for a scalar/enum type t
that does not implement EnumValues or EnumOptionProvider itself.


```go
func WithRootLabel(label string) Option
```
WithRootLabel sets the display label of the root node.


```go
func WithValidator(fn func(target any) error) Option
```
WithValidator registers an additional validation callback invoked with the
working copy pointer during Validate and Commit.




### Type ScalarKind
```go
type ScalarKind int
```
ScalarKind identifies the broad scalar category of a type for widget
selection and formatting.

### Constants
### ScalarUnknown, ScalarString, ScalarBool, ScalarInt, ScalarUint, ScalarFloat, ScalarDuration, ScalarText, ScalarFlag, ScalarEnum, ScalarCustom
```go
const (
	ScalarUnknown ScalarKind = iota
	ScalarString
	ScalarBool
	ScalarInt
	ScalarUint
	ScalarFloat
	ScalarDuration
	ScalarText
	ScalarFlag
	ScalarEnum
	ScalarCustom
)
```




### Type Schema
```go
type Schema struct {
	Root   *TypeDesc
	Codecs *CodecRegistry
}
```
Schema holds the root TypeDesc and the CodecRegistry used to construct it.

### Functions

```go
func BuildSchema(t reflect.Type, codecs *CodecRegistry) (*Schema, error)
```
BuildSchema reflects on t (which may be a struct or pointer to struct) and
builds a cyclic-safe Schema using codecs.


```go
func BuildSchemaWithEnums(t reflect.Type, codecs *CodecRegistry, enumOverrides map[reflect.Type][]string) (*Schema, error)
```
BuildSchemaWithEnums builds a Schema with optional explicit enum
option lists for external types that do not implement EnumValues or
EnumOptionProvider.




### Type TypeDesc
```go
type TypeDesc struct {
	Kind        Kind
	Type        reflect.Type
	ScalarKind  ScalarKind
	EnumOptions []string     // populated for enum KindScalar types
	Fields      []*FieldDesc // populated for KindStruct
	Key         *TypeDesc    // populated for KindMap
	Elem        *TypeDesc    // populated for KindSlice, KindArray, KindMap, KindPtr
}
```
TypeDesc describes a Go type in the schema graph. Recursive types form
cycles via Elem or FieldDesc.Type pointers.


### Type Validator
```go
type Validator interface {
	Validate() error
}
```
Validator may be implemented by configuration structs to provide custom
validation rules checked during Document.Validate and Document.Commit.


### Type ValueRef
```go
type ValueRef interface {
	Type() reflect.Type
	Get() reflect.Value
	Set(v reflect.Value) error
	DirectAddr() bool
}
```
ValueRef provides uniform read/write access to a Go value regardless of
whether it lives in a struct field, slice element, pointer, or unaddressable
map entry.





