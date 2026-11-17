# Package [cloudeng.io/tui/tview](https://pkg.go.dev/cloudeng.io/tui/tview?tab=doc)

```go
import cloudeng.io/tui/tview
```

Packages for building `rivo/tview` TUI components, including a tag-driven settings editor for arbitrary Go structs:

- `structtag`: parses `yaml` struct tags with `tview` tag overrides (`label`, `widget`, `options`, `always-hide`, `hide-with-show`, `readonly`, `inline`, `itemlabel`, `help`, `order`, `show`/`-`).
- `structmodel`: UI-agnostic reflection schema builder (with cycle detection for recursive structs), uniform `ValueRef` bindings (handling structs, pointers, slices, arrays, and unaddressable map values), scalar codecs (`time.Duration`, `encoding.TextMarshaler`/`TextUnmarshaler`), and transactional `Commit`/`Revert`.
- `structedit`: split-view `rivo/tview` editor (`TreeView` + `Form` / slice & map `Table` editors) over `structmodel`.
