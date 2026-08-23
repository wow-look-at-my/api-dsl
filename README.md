# api-dsl

The shared lexical layer behind this org's declarative XML API tools: an order-preserving XML DOM, a compiler that turns inline placeholders into Go `text/template` source, and the renderer that executes it.

It carries no vocabulary of its own. It knows `<value>`, `<if>`, `<for>` and `<else>`, and nothing about commands, requests, caches, or routes. Each consumer names its own elements, walks the DOM, and calls `CompileContent` on the content it wants treated as a template.

## Consumers

- [api-cli](https://github.com/wow-look-at-my/api-cli) — a declarative CLI: an XML config becomes a cobra command tree that runs commands and HTTP requests.
- api-mirror — a declarative caching proxy: an XML spec becomes a SQLite schema, HTTP routes, and webhook handlers.

## Install

```sh
go get github.com/wow-look-at-my/api-dsl
```

## The language

Placeholders live inside ordinary element content, mixed with text:

```xml
<url>https://api.github.com/repos/<value name="arg.repo" as="urlpath"/>/pulls</url>
<header name="Authorization">Bearer <value name="env.GITHUB_TOKEN"/></header>
<query>
	<if test="flag.state">state=<value name="flag.state" default="open"/></if>
	<for each="flag.labels">&labels=<value expr="{{ . }}"/></for>
</query>
```

That content compiles to template source, which renders against the consumer's data:

```go
root, err := apidsl.ParseDOM(src)                 // order-preserving DOM
tmpl, err := apidsl.CompileContent(root)          // "Bearer {{ .env.GITHUB_TOKEN }}"
out, err := apidsl.RenderString(tmpl, data)       // "Bearer ghp_..."
```

## Adding your own template functions

`FuncMap` is the shared set: sprig plus `truthy`, `querystring`, `urlpath` and `repeatkey`. A consumer that needs more builds a `Renderer` instead of adding to the shared set:

```go
r := apidsl.NewRenderer(template.FuncMap{"shellquote": shellQuote})
out, err := r.Render(tmpl, data)
```

## Build

`go-toolchain` runs tidy, vet, tests with coverage, and the build. Never run bare `go` commands.
