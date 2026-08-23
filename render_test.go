package apidsl

import (
	"encoding/json"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderString_Namespaces(t *testing.T) {
	data := map[string]any{
		"var":  map[string]any{"base": "https://api.example.com"},
		"arg":  map[string]any{"id": 7},
		"env":  map[string]string{"TOKEN": "t0k"},
		"data": map[string]any{"items": []any{"a", "b"}},
	}
	out, err := RenderString("{{.var.base}}/u/{{.arg.id}}?t={{.env.TOKEN}}&n={{len .data.items}}", data)
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/u/7?t=t0k&n=2", out)
}

func TestRenderString_MissingKeyZero(t *testing.T) {
	out, err := RenderString("[{{.var.nope}}]", map[string]any{"var": map[string]any{}})
	require.NoError(t, err)
	assert.Equal(t, "[<no value>]", out)

	out, err = RenderString("[{{.env.NOPE}}]", map[string]any{"env": map[string]string{}})
	require.NoError(t, err)
	assert.Equal(t, "[]", out)
}

func TestRenderString_Errors(t *testing.T) {
	_, err := RenderString("{{ .unclosed ", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse template")

	_, err = RenderString(`{{ if not .set }}{{ fail "need it" }}{{ end }}`, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "execute template")
	assert.Contains(t, err.Error(), "need it")
}

func TestFuncMap_CarriesSharedHelpers(t *testing.T) {
	fm := FuncMap()
	for _, name := range []string{"truthy", "querystring", "urlpath", "repeatkey", "upper", "default"} {
		assert.Contains(t, fm, name, name)
	}
}

func TestFuncMap_ReturnsAFreshMap(t *testing.T) {
	first := FuncMap()
	first["mine"] = func() string { return "x" }
	assert.NotContains(t, FuncMap(), "mine")
}

func TestNewRenderer_AddsConsumerFuncs(t *testing.T) {
	r := NewRenderer(template.FuncMap{"shout": func(s string) string { return s + "!" }})
	out, err := r.Render(`{{ shout "hi" }}/{{ if truthy .on }}on{{ end }}`, map[string]any{"on": "yes"})
	require.NoError(t, err)
	assert.Equal(t, "hi!/on", out)

	// The added function stays out of the shared set.
	_, err = RenderString(`{{ shout "hi" }}`, nil)
	assert.Error(t, err)
}

func TestNewRenderer_LastExtraWins(t *testing.T) {
	r := NewRenderer(
		template.FuncMap{"pick": func() string { return "first" }},
		template.FuncMap{"pick": func() string { return "second" }},
	)
	out, err := r.Render("{{ pick }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "second", out)
}

func TestRenderer_FuncsIsACopy(t *testing.T) {
	r := NewRenderer()
	fm := r.Funcs()
	assert.Contains(t, fm, "truthy")
	delete(fm, "truthy")
	assert.Contains(t, r.Funcs(), "truthy")
}

func TestQueryString(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"empty map", map[string]any{}, ""},
		{"string map", map[string]string{"a": "1"}, "?a=1"},
		{"string map drops empty", map[string]string{"a": "1", "b": ""}, "?a=1"},
		{"any map sorts keys", map[string]any{"b": "2", "a": "1"}, "?a=1&b=2"},
		{"slice repeats key", map[string]any{"tag": []any{"a", "b"}}, "?tag=a&tag=b"},
		{"string slice", map[string]any{"tag": []string{"a", "", "b"}}, "?tag=a&tag=b"},
		{"bool and numbers", map[string]any{"ok": true, "n": 3, "f": 1.5}, "?f=1.5&n=3&ok=true"},
		{"json number", map[string]any{"n": json.Number("42")}, "?n=42"},
		{"nil value dropped", map[string]any{"a": nil, "b": "1"}, "?b=1"},
		{"escapes", map[string]any{"q": "a b&c"}, "?q=a+b%26c"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := queryString(c.in)
			require.NoError(t, err)
			assert.Equal(t, c.want, out)
		})
	}
}

func TestQueryString_Errors(t *testing.T) {
	_, err := queryString("nope")
	assert.Error(t, err)
	_, err = queryString(map[string]any{"k": struct{}{}})
	assert.Error(t, err)
	_, err = queryString(map[string]any{"k": []any{struct{}{}}})
	assert.Error(t, err)
}

func TestQueryString_ViaTemplate(t *testing.T) {
	out, err := RenderString("/u{{querystring .q}}", map[string]any{"q": map[string]any{"page": "2"}})
	require.NoError(t, err)
	assert.Equal(t, "/u?page=2", out)
}

func TestRepeatKey(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string slice", []string{"a", "b"}, "tag=a&tag=b"},
		{"any slice", []any{1, "b"}, "tag=1&tag=b"},
		{"single string", "a", "tag=a"},
		{"drops empty", []string{"a", "", "b"}, "tag=a&tag=b"},
		{"escapes", []string{"a b"}, "tag=a+b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := repeatKey("tag", c.in)
			require.NoError(t, err)
			assert.Equal(t, c.want, out)
		})
	}
	_, err := repeatKey("tag", 5)
	assert.Error(t, err)
}

func TestRepeatKey_ViaTemplate(t *testing.T) {
	out, err := RenderString(`?{{repeatkey "tag" .tags}}`, map[string]any{"tags": []string{"x", "y"}})
	require.NoError(t, err)
	assert.Equal(t, "?tag=x&tag=y", out)
}

func TestUrlpath_ViaTemplate(t *testing.T) {
	out, err := RenderString("/repos/{{urlpath .name}}", map[string]any{"name": "a/b c"})
	require.NoError(t, err)
	assert.Equal(t, "/repos/a%2Fb%20c", out)
}

func TestEnvMap(t *testing.T) {
	t.Setenv("APIDSL_TEST_VAR", "hello")
	env := EnvMap()
	assert.Equal(t, "hello", env["APIDSL_TEST_VAR"])
	assert.NotContains(t, env, "")
}

func TestIsTruthy(t *testing.T) {
	for _, s := range []string{"yes", "1", "true", "anything", " x "} {
		assert.True(t, IsTruthy(s), s)
	}
	for _, s := range []string{"", "   ", "false", "FALSE", "0", "no", "No"} {
		assert.False(t, IsTruthy(s), s)
	}
}

func TestTruthy(t *testing.T) {
	assert.False(t, Truthy(nil))
	assert.False(t, Truthy(false))
	assert.True(t, Truthy(true))
	assert.False(t, Truthy("no"))
	assert.True(t, Truthy("yes"))
	assert.False(t, Truthy(0))
	assert.True(t, Truthy(3))
	assert.True(t, Truthy([]string{"a"}))
}

func TestLookupPath(t *testing.T) {
	data := map[string]any{
		"repository": map[string]any{"owner": map[string]any{"login": "octo"}, "id": 5},
		"env":        map[string]string{"TOKEN": "t"},
		"scalar":     "s",
	}
	assert.Equal(t, "octo", LookupPath(data, "repository.owner.login"))
	assert.Equal(t, 5, LookupPath(data, "repository.id"))
	assert.Equal(t, "t", LookupPath(data, "env.TOKEN"))
	assert.Equal(t, data, LookupPath(data, ""))
	assert.Equal(t, "octo", LookupPath(data, " repository . owner . login "))
	assert.Nil(t, LookupPath(data, "repository.missing"))
	assert.Nil(t, LookupPath(data, "scalar.deeper"))
	assert.Nil(t, LookupPath(nil, "a"))
}

func TestMergeVars(t *testing.T) {
	parent := map[string]any{"a": 1, "b": 2}
	child := map[string]any{"b": 3, "c": 4}
	out := MergeVars(parent, child)
	assert.Equal(t, map[string]any{"a": 1, "b": 3, "c": 4}, out)
	// The inputs are untouched.
	assert.Equal(t, 2, parent["b"])
	assert.Len(t, child, 2)

	assert.Equal(t, map[string]any{"a": 1}, MergeVars(nil, map[string]any{"a": 1}))
	assert.Equal(t, map[string]any{"a": 1}, MergeVars(map[string]any{"a": 1}, nil))
	assert.Empty(t, MergeVars(nil, nil))
}

func TestSprigHelpersAvailable(t *testing.T) {
	out, err := RenderString(`{{ upper .s }}-{{ .list | join "," }}-{{ .n | default 9 }}`, map[string]any{
		"s":    "hi",
		"list": []string{"a", "b"},
	})
	require.NoError(t, err)
	assert.Equal(t, "HI-a,b-9", out)
}
