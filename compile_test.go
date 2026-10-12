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
		{"value expr element", "<t><value>\n\t<expr>\n\t\t{{ upper .env.Z }}\n\t</expr>\n</value></t>", `{{ upper .env.Z }}`},
		{"value expr element with CDATA", `<t><value><expr><![CDATA[{{ upper .env.Z }}]]></expr></value></t>`, `{{ upper .env.Z }}`},
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
		"value expr in both forms":  `<t><value expr="{{ . }}"><expr>{{ . }}</expr></value></t>`,
		"value expr element twice":  `<t><value><expr>a</expr><expr>b</expr></value></t>`,
		"value name and expr elem":  `<t><value name="a"><expr>b</expr></value></t>`,
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

func templateText(t *testing.T, src string) string {
	t.Helper()
	out, err := TemplateText(mustDOM(t, src))
	require.NoError(t, err)
	return out
}

func TestTemplateText_IndentedLinesConcatenate(t *testing.T) {
	src := "<when>\n\t\t{{ and\n\t\t\t(eq .a 1)\n\t\t\t(eq .b 2) }}\n\t</when>"
	assert.Equal(t, "{{ and(eq .a 1)(eq .b 2) }}", templateText(t, src))
}

func TestTemplateText_InlineWhitespaceStays(t *testing.T) {
	assert.Equal(t, "{{ .a }} and {{ .b }}", templateText(t, "<t>  {{ .a }} and {{ .b }}  </t>"))
}

func TestTemplateText_CDATAIsVerbatim(t *testing.T) {
	src := "<t><![CDATA[\n\tline one\n  line two \n]]></t>"
	assert.Equal(t, "\n\tline one\n  line two \n", templateText(t, src))
}

func TestTemplateText_MixedTextAndCDATA(t *testing.T) {
	src := "<t>\n\t{{ .a }}\n\t<![CDATA[ - ]]>\n\t{{ .b }}\n</t>"
	assert.Equal(t, "{{ .a }} - {{ .b }}", templateText(t, src))
}

func TestTemplateText_EntitiesDecode(t *testing.T) {
	assert.Equal(t, `{{ if lt .a 1 }}&{{ end }}`, templateText(t, `<t>{{ if lt .a 1 }}&amp;{{ end }}</t>`))
}

func TestTemplateText_Errors(t *testing.T) {
	_, err := TemplateText(mustDOM(t, `<t><value name="a"/></t>`))
	require.ErrorContains(t, err, "unexpected child element <value>")

	_, err = TemplateText(mustDOM(t, `<t bad="1">x</t>`))
	require.ErrorContains(t, err, `unknown attribute "bad"`)
}

func TestTemplateAttrs_AttributeAndElementAgree(t *testing.T) {
	attr := mustDOM(t, `<step when="{{ eq .a 1 }}" name="s"/>`)
	elem := mustDOM(t, "<step name=\"s\">\n\t<when>\n\t\t{{ eq .a 1 }}\n\t</when>\n</step>")

	fromAttr, _, err := TemplateAttrs(attr, "when", "over")
	require.NoError(t, err)
	fromElem, rest, err := TemplateAttrs(elem, "when", "over")
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"when": "{{ eq .a 1 }}"}, fromAttr)
	assert.Equal(t, fromAttr, fromElem)
	assert.Empty(t, rest.Children())
	assert.Equal(t, "s", rest.Attr("name"))
}

func TestTemplateAttrs_RestKeepsOtherContent(t *testing.T) {
	n := mustDOM(t, `<output><path>out.txt</path>body <value name="a"/></output>`)
	tmpl, rest, err := TemplateAttrs(n, "path")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"path": "out.txt"}, tmpl)

	body, err := CompileContent(rest)
	require.NoError(t, err)
	assert.Equal(t, "body {{ .a }}", body)
}

func TestTemplateAttrs_EmptyAttributeIsSet(t *testing.T) {
	tmpl, _, err := TemplateAttrs(mustDOM(t, `<flag default=""/>`), "default")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"default": ""}, tmpl)
}

func TestTemplateAttrs_Errors(t *testing.T) {
	_, _, err := TemplateAttrs(mustDOM(t, `<step when="a"><when>b</when></step>`), "when")
	require.ErrorContains(t, err, "<step>: when= and <when> both set the same template")

	_, _, err = TemplateAttrs(mustDOM(t, `<step><when>a</when><when>b</when></step>`), "when")
	require.ErrorContains(t, err, "<step>: <when> is set more than once")

	_, _, err = TemplateAttrs(mustDOM(t, `<step><when><value name="a"/></when></step>`), "when")
	require.ErrorContains(t, err, "<when>: unexpected child element <value>")
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
