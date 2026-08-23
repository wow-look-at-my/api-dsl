package apidsl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compileSrc parses one element and compiles its content to template source.
func compileSrc(t *testing.T, src string) string {
	t.Helper()
	out, err := CompileContent(mustDOM(t, src))
	require.NoError(t, err)
	return out
}

// These are the compiled forms every consumer depends on. A change here
// changes what every shipped document means.
func TestCompileContent_PlaceholderForms(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"value name", `<t><value name="var.base"/></t>`, `{{ .var.base }}`},
		{"value default and as", `<t><value name="env.X" default="def" as="urlpath"/></t>`, `{{ urlpath (.env.X | default "def") }}`},
		{"value default only", `<t><value name="env.X" default="def"/></t>`, `{{ .env.X | default "def" }}`},
		{"value expr", `<t><value expr="{{ upper .env.Z }}"/></t>`, `{{ upper .env.Z }}`},
		{"if eq with else", `<t><if test="env.Y" eq="1">yes<else/>no</if></t>`, `{{ if eq (printf "%v" .env.Y) "1" }}yes{{ else }}no{{ end }}`},
		{"if without else", `<t><if test="env.W">on</if></t>`, `{{ if truthy .env.W }}on{{ end }}`},
		{"for each", `<t><for each="items"><value name="name"/></for></t>`, `{{ range .items }}{{ .name }}{{ end }}`},
		{"nested for and if", `<t><for each="items"><if test="on">x</if></for></t>`, `{{ range .items }}{{ if truthy .on }}x{{ end }}{{ end }}`},
		{"literal text around a value", `<t>Bearer <value name="var.token"/></t>`, `Bearer {{ .var.token }}`},
		{"dashed path uses index", `<t><value name="flag.dry-run"/></t>`, `{{ (index . "flag" "dry-run") }}`},
		{"empty eq is an equality test", `<t><if test="flag.s" eq="">empty<else/>set</if></t>`, `{{ if eq (printf "%v" .flag.s) "" }}empty{{ else }}set{{ end }}`},
		{"structural newlines are dropped", "<t>\n\t<value name=\"a\"/>\n</t>", `{{ .a }}`},
		{"inline whitespace is kept", `<t> <value name="a"/> </t>`, ` {{ .a }} `},
		{"empty element", `<t/>`, ``},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, compileSrc(t, c.src))
		})
	}
}

func TestCompileContent_CompiledFormsRender(t *testing.T) {
	dashed := compileSrc(t, `<t><value name="flag.dry-run"/></t>`)
	out, err := RenderString(dashed, map[string]any{"flag": map[string]any{"dry-run": "yes"}})
	require.NoError(t, err)
	assert.Equal(t, "yes", out)

	loop := compileSrc(t, `<t><for each="items"><value name="name"/>,</for></t>`)
	out, err = RenderString(loop, map[string]any{
		"items": []any{map[string]any{"name": "a"}, map[string]any{"name": "b"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "a,b,", out)

	scalar := compileSrc(t, `<t><for each="tags"><value expr="{{ . }}"/>;</for></t>`)
	out, err = RenderString(scalar, map[string]any{"tags": []any{"x", "y"}})
	require.NoError(t, err)
	assert.Equal(t, "x;y;", out)

	cond := compileSrc(t, `<t><if test="on">yes<else/>no</if></t>`)
	out, err = RenderString(cond, map[string]any{"on": "false"})
	require.NoError(t, err)
	assert.Equal(t, "no", out)
}

func TestCompileContent_MultiLineBodyDedentsTabsOnly(t *testing.T) {
	tabs := compileSrc(t, "<t>\n\t\tfirst\n\t\t\tsecond\n\t\t</t>")
	assert.Equal(t, "first\n\tsecond", tabs)

	// A space is content, never indentation, so nothing is removed.
	spaces := compileSrc(t, "<t>\n  first\n  second\n</t>")
	assert.Equal(t, "  first\n  second", spaces)

	// A blank line inside the block does not lower the common indent.
	blank := compileSrc(t, "<t>\n\t\tfirst\n\n\t\tsecond\n\t</t>")
	assert.Equal(t, "first\n\nsecond", blank)
}

func TestCompileContent_Errors(t *testing.T) {
	cases := map[string]string{
		"value with name and expr":  `<t><value name="a" expr="b"/></t>`,
		"value with neither":        `<t><value/></t>`,
		"value expr with default":   `<t><value expr="{{ . }}" default="d"/></t>`,
		"value expr with as":        `<t><value expr="{{ . }}" as="urlpath"/></t>`,
		"value unknown attribute":   `<t><value name="a" bogus="1"/></t>`,
		"if without test":           `<t><if>x</if></t>`,
		"if with empty test":        `<t><if test="">x</if></t>`,
		"if unknown attribute":      `<t><if test="a" bogus="1">x</if></t>`,
		"two else elements":         `<t><if test="a">x<else/>y<else/>z</if></t>`,
		"else with an attribute":    `<t><if test="a">x<else bogus="1"/>y</if></t>`,
		"for without each":          `<t><for>x</for></t>`,
		"for unknown attribute":     `<t><for each="i" bogus="1">x</for></t>`,
		"unexpected element":        `<t><bogus/></t>`,
		"bad element inside if":     `<t><if test="a"><bogus/></if></t>`,
		"bad element inside else":   `<t><if test="a">x<else/><bogus/></if></t>`,
		"bad element inside for":    `<t><for each="i"><bogus/></for></t>`,
		"bad nested value in a for": `<t><for each="i"><value/></for></t>`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := CompileContent(mustDOM(t, src))
			assert.Error(t, err)
		})
	}
}

func TestCompileTextElem(t *testing.T) {
	out, err := CompileTextElem(mustDOM(t, `<url><value name="var.base"/>/p</url>`))
	require.NoError(t, err)
	assert.Equal(t, `{{ .var.base }}/p`, out)

	_, err = CompileTextElem(mustDOM(t, `<url bad="1">u</url>`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown attribute "bad"`)
}

func TestTextOf(t *testing.T) {
	out, err := TextOf(mustDOM(t, "<description>\n\tHi there\n</description>"))
	require.NoError(t, err)
	assert.Equal(t, "Hi there", out)

	empty, err := TextOf(mustDOM(t, `<description/>`))
	require.NoError(t, err)
	assert.Equal(t, "", empty)

	_, err = TextOf(mustDOM(t, `<description>a<value name="x"/></description>`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected child element <value>")
}

func TestIsPlaceholder(t *testing.T) {
	for _, name := range []string{"value", "if", "for", "else"} {
		assert.True(t, IsPlaceholder(name), name)
	}
	for _, name := range []string{"command", "run", "request", "", "Value"} {
		assert.False(t, IsPlaceholder(name), name)
	}
}

func TestDotPath(t *testing.T) {
	cases := map[string]string{
		"var.base_url": ".var.base_url",
		"items":        ".items",
		"a.b2.c":       ".a.b2.c",
		" var.x ":      ".var.x",
		"flag.dry-run": `(index . "flag" "dry-run")`,
		"a.2b":         `(index . "a" "2b")`,
		"a..b":         `(index . "a" "" "b")`,
		"":             `(index . "")`,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, DotPath(in))
		})
	}
}

func TestDotPath_IndexFormReadsBothMapKinds(t *testing.T) {
	src := DotPath("env.API-KEY")
	out, err := RenderString("{{ "+src+" }}", map[string]any{"env": map[string]string{"API-KEY": "k"}})
	require.NoError(t, err)
	assert.Equal(t, "k", out)
}
