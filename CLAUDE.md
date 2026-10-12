# api-dsl — repo orientation for Claude

A shared Go module (`github.com/wow-look-at-my/api-dsl`, package `apidsl`) holding the *lexical* layer of a declarative XML API language: parse, compile, render. Two consumers build their own vocabulary on it — `api-cli` (declarative CLI) and `api-mirror` (declarative caching proxy).

The language itself is documented for humans in `README.md`.

## File map

| File             | Role                                                                                                      |
|------------------|-----------------------------------------------------------------------------------------------------------|
| `doc.go`         | Package doc: the three parts and the split of responsibility with a consumer.                              |
| `dom.go`         | `Node`, `Attr`, `Item`; `ParseDOM`, `StripXMLDecl`, `CheckAttrs`. Order-preserving DOM over `encoding/xml`. |
| `compile.go`     | `CompileContent`, `CompileTextElem`, `TextOf`, `TemplateText`, `TemplateAttrs`, `IsPlaceholder`, `DotPath`. Placeholders to template source, and a template attribute's element form. |
| `render.go`      | `RenderString`, `Renderer`, `FuncMap`, `LookupPath`, `MergeVars`, `EnvMap`, `Truthy`, `IsTruthy`.           |
| `*_test.go`      | One test file per source file. The compiled-form table in `compile_test.go` is the pinned contract.        |

## Invariants

- **No consumer vocabulary lives here.** The placeholder names (`value`, `if`, `for`, `else`) are the whole vocabulary this module owns. An element name, an attribute name, or an error message naming a consumer's root element belongs in that consumer.
- **`ParseDOM` does not check the root element's name.** The caller does, because each consumer has a different one.
- **Compiled forms are a contract.** The table in `TestCompileContent_PlaceholderForms` pins each placeholder's exact template output. Changing an entry changes what every shipped document means, in both consumers.
- **`truthy` is not optional.** `<if test=>` compiles to it, so a func map without it fails to execute.
- **A consumer adds template functions through `NewRenderer`, never through `FuncMap`.** `FuncMap` returns a fresh map each call, and the shared set stays free of one consumer's helpers.
- **Templates render with `missingkey=zero`.** Do not change this default; documents rely on a missing key rendering empty.
- **Structural indentation is tabs.** `dedentTabs` counts leading tabs only, so a leading space is content and survives the dedent.
- **`CheckAttrs` on every element a consumer builds.** An unread attribute is a typo that must fail the load.

## Tooling

- `go-toolchain` runs tidy, vet, tests with coverage, and the build. **Always `go-toolchain`, never bare `go ...`.** Coverage minimum is 80%.
- Vet rejects a `map[...]bool` used as a set. Use a `switch` or `slices.Contains` here rather than pulling in a container dependency.
- CI: `.github/workflows/ci.yml`, one `test` job. Never name a job `all-builds`.

## Conventions

- Tabs for indentation. Testify style: `require` for must-stop, `assert` for values, `t.Helper()` in helpers.
- Comments state what the code does now, in short active sentences. No changelog prose.
- Branch naming for Claude sessions: `claude/<descriptor>-<short-id>`.
