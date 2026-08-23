package apidsl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

// FuncMap returns the template functions every consumer of the language gets:
// sprig's text FuncMap plus the helpers a placeholder compiles to. The result
// is a fresh map, so a caller may add to it.
//
// `truthy` is not optional. It is the function <if test=> compiles to, and a
// template that loses it fails to execute.
func FuncMap() template.FuncMap {
	fm := sprig.TxtFuncMap()
	fm["querystring"] = queryString
	fm["urlpath"] = url.PathEscape
	fm["repeatkey"] = repeatKey
	fm["truthy"] = Truthy
	return fm
}

// Renderer executes templates with the shared function set plus a consumer's
// own functions. It is the extension point: this module holds no vocabulary
// for any one consumer, so a consumer that needs a helper of its own supplies
// it here instead of adding it to FuncMap.
//
// A Renderer is read-only after construction, and safe for concurrent use.
type Renderer struct{ funcs template.FuncMap }

// NewRenderer returns a Renderer over FuncMap plus each extra map, applied in
// order. A later map wins on a name collision, so a consumer can replace a
// shared function on purpose. Replacing `truthy` changes what every <if test=>
// in that consumer's documents means.
func NewRenderer(extra ...template.FuncMap) *Renderer {
	fm := FuncMap()
	for _, m := range extra {
		for k, v := range m {
			fm[k] = v
		}
	}
	return &Renderer{funcs: fm}
}

// Funcs returns a copy of the renderer's function set.
func (r *Renderer) Funcs() template.FuncMap {
	out := make(template.FuncMap, len(r.funcs))
	for k, v := range r.funcs {
		out[k] = v
	}
	return out
}

// Render executes a text/template against data using the renderer's functions.
//
// missingkey=zero: a missing map key yields the zero value of the map's
// element type. A document that must stop on a missing value says so itself,
// with sprig's `fail`.
func (r *Renderer) Render(tmpl string, data any) (string, error) {
	t, err := template.New("t").Funcs(r.funcs).Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template %q: %w", tmpl, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %q: %w", tmpl, err)
	}
	return buf.String(), nil
}

// defaultRenderer backs RenderString. It carries the shared functions only.
var defaultRenderer = NewRenderer()

// RenderString executes a text/template against data using FuncMap. A consumer
// with functions of its own builds a Renderer and calls Render instead.
func RenderString(tmpl string, data any) (string, error) {
	return defaultRenderer.Render(tmpl, data)
}

// EnvMap returns the process environment as a map[string]string, for use as
// {{.env.VAR}} inside a template.
func EnvMap() map[string]string {
	out := make(map[string]string, len(os.Environ()))
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 {
			out[kv[:i]] = kv[i+1:]
		}
	}
	return out
}

// IsTruthy reads a rendered string as a boolean. Empty, "false", "0" and "no"
// are false. Everything else is true.
func IsTruthy(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return false
	}
	switch strings.ToLower(t) {
	case "false", "0", "no":
		return false
	}
	return true
}

// Truthy reports whether v is truthy under IsTruthy's rules. nil and false are
// falsy, a string goes through IsTruthy, and anything else is stringified
// first. It is the `truthy` template function, which <if test=> compiles to.
func Truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return IsTruthy(x)
	default:
		return IsTruthy(fmt.Sprintf("%v", x))
	}
}

// LookupPath walks a dotted path ("repository.owner.login") into decoded JSON.
// A missing segment, or a segment that traverses a non-map, returns nil. An
// empty path returns the value unchanged.
func LookupPath(data any, path string) any {
	cur := data
	for _, seg := range strings.Split(path, ".") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		switch m := cur.(type) {
		case map[string]any:
			cur = m[seg]
		case map[string]string:
			cur = m[seg]
		default:
			return nil
		}
	}
	return cur
}

// MergeVars merges child into a copy of parent. The child wins on a collision.
// A nil input counts as an empty map, and the result is always a fresh map.
func MergeVars(parent, child map[string]any) map[string]any {
	out := make(map[string]any, len(parent)+len(child))
	for k, v := range parent {
		out[k] = v
	}
	for k, v := range child {
		out[k] = v
	}
	return out
}

// queryString renders a map as a URL query string, leading "?" included. A
// value that renders empty is dropped, so an unset optional parameter does not
// clutter the URL. A slice value repeats its key.
func queryString(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	values := url.Values{}
	switch m := v.(type) {
	case map[string]string:
		for k, val := range m {
			if val != "" {
				values.Add(k, val)
			}
		}
	case map[string]any:
		for k, val := range m {
			if err := addQueryValue(values, k, val); err != nil {
				return "", err
			}
		}
	default:
		return "", fmt.Errorf("querystring: unsupported type %T (need map)", v)
	}
	enc := values.Encode()
	if enc == "" {
		return "", nil
	}
	return "?" + enc, nil
}

func addQueryValue(values url.Values, key string, v any) error {
	switch val := v.(type) {
	case nil:
		return nil
	case string:
		if val != "" {
			values.Add(key, val)
		}
	case bool:
		values.Add(key, fmt.Sprintf("%t", val))
	case json.Number:
		values.Add(key, val.String())
	case int, int64, float64:
		values.Add(key, fmt.Sprintf("%v", val))
	case []any:
		for _, item := range val {
			if err := addQueryValue(values, key, item); err != nil {
				return err
			}
		}
	case []string:
		for _, item := range val {
			if item != "" {
				values.Add(key, item)
			}
		}
	default:
		return fmt.Errorf("querystring: unsupported value type %T for key %q", v, key)
	}
	return nil
}

// repeatKey emits repeated query parameters for one key, one per slice
// element, URL-encoded and with no leading "?".
func repeatKey(key string, v any) (string, error) {
	var parts []string
	switch x := v.(type) {
	case nil:
		return "", nil
	case []string:
		parts = x
	case []any:
		for _, item := range x {
			parts = append(parts, fmt.Sprintf("%v", item))
		}
	case string:
		parts = []string{x}
	default:
		return "", fmt.Errorf("repeatkey: unsupported type %T (need list)", v)
	}
	values := url.Values{}
	for _, p := range parts {
		if p != "" {
			values.Add(key, p)
		}
	}
	return values.Encode(), nil
}
