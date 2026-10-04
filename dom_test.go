package apidsl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustDOM parses src and requires that it succeeds.
func mustDOM(t *testing.T, src string) *Node {
	t.Helper()
	n, err := ParseDOM([]byte(src))
	require.NoError(t, err)
	return n
}

func TestParseDOM_MixedContentKeepsOrder(t *testing.T) {
	root := mustDOM(t, `<r>a<b/>c<d/>e</r>`)
	assert.Equal(t, "r", root.Name())
	items := root.Content()
	require.Len(t, items, 5)
	assert.Equal(t, "a", items[0].Text)
	assert.True(t, items[0].IsText())
	require.NotNil(t, items[1].Elem)
	assert.False(t, items[1].IsText())
	assert.Equal(t, "b", items[1].Elem.Name())
	assert.Equal(t, "c", items[2].Text)
	assert.Equal(t, "d", items[3].Elem.Name())
	assert.Equal(t, "e", items[4].Text)

	kids := root.Children()
	require.Len(t, kids, 2)
	assert.Equal(t, "b", kids[0].Name())
	assert.Equal(t, "d", kids[1].Name())
}

func TestParseDOM_ChildrenOfTextOnlyElementIsEmpty(t *testing.T) {
	root := mustDOM(t, `<r>only text</r>`)
	assert.Empty(t, root.Children())
	require.Len(t, root.Content(), 1)
}

func TestParseDOM_CDATAIsTextMarkedAsCDATA(t *testing.T) {
	root := mustDOM(t, `<r><![CDATA[ <raw> & "quoted" ]]></r>`)
	require.Len(t, root.Content(), 1)
	assert.Equal(t, ` <raw> & "quoted" `, root.Content()[0].Text)
	assert.True(t, root.Content()[0].IsText())
	assert.True(t, root.Content()[0].CDATA)
}

func TestParseDOM_CDATAIsItsOwnChunk(t *testing.T) {
	root := mustDOM(t, "<r>a &amp; <![CDATA[b ]]>c<x/><![CDATA[d]]></r>")
	items := root.Content()
	require.Len(t, items, 5)
	assert.Equal(t, Item{Text: "a & "}, items[0])
	assert.Equal(t, Item{Text: "b ", CDATA: true}, items[1])
	assert.Equal(t, Item{Text: "c"}, items[2])
	assert.Equal(t, "x", items[3].Elem.Name())
	assert.Equal(t, Item{Text: "d", CDATA: true}, items[4])
}

func TestParseDOM_CDATAAfterTheDeclaration(t *testing.T) {
	root := mustDOM(t, "<?xml version=\"1.1\"?>\n<r><![CDATA[x]]></r>")
	require.Len(t, root.Content(), 1)
	assert.True(t, root.Content()[0].CDATA)
}

func TestParseDOM_SkipsSchemaInstanceAttributes(t *testing.T) {
	root := mustDOM(t, `<r xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="r.xsd" name="n"/>`)
	assert.Equal(t, []Attr{{"name", "n"}}, root.Attrs())
	assert.NoError(t, CheckAttrs(root, "name"))
}

func TestParseDOM_AttributesInDocumentOrder(t *testing.T) {
	root := mustDOM(t, `<r b="2" a="1" empty=""/>`)
	assert.Equal(t, []Attr{{"b", "2"}, {"a", "1"}, {"empty", ""}}, root.Attrs())
	assert.Equal(t, "1", root.Attr("a"))
	assert.Equal(t, "", root.Attr("missing"))
	assert.True(t, root.HasAttr("empty"))
	assert.False(t, root.HasAttr("missing"))
}

func TestParseDOM_RejectsDuplicateAttribute(t *testing.T) {
	_, err := ParseDOM([]byte(`<r a="1" a="2"/>`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `duplicate attribute "a"`)
}

func TestParseDOM_SkipsNamespaceDeclarations(t *testing.T) {
	root := mustDOM(t, `<r xmlns="urn:x" xmlns:p="urn:y" name="n"/>`)
	assert.Equal(t, []Attr{{"name", "n"}}, root.Attrs())
	assert.NoError(t, CheckAttrs(root, "name"))
}

func TestParseDOM_DropsCommentsAndProcessingInstructions(t *testing.T) {
	root := mustDOM(t, `<r><!-- gone --><?target data?><b/></r>`)
	require.Len(t, root.Children(), 1)
	assert.Equal(t, "b", root.Children()[0].Name())
}

func TestParseDOM_Errors(t *testing.T) {
	cases := map[string]string{
		"empty document": ``,
		"only a comment": `<!-- nothing -->`,
		"malformed":      `<r><b></r>`,
		"unclosed":       `<r>`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseDOM([]byte(src))
			assert.Error(t, err)
		})
	}
}

// The message must not name any one consumer's root element.
func TestParseDOM_MultipleRootsErrorIsGeneric(t *testing.T) {
	_, err := ParseDOM([]byte(`<a/><b/>`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple top-level elements")
	assert.NotContains(t, err.Error(), "config")
}

func TestStripXMLDecl(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bom and declaration", "\xef\xbb\xbf<?xml version=\"1.1\"?>\n<r/>", "\n<r/>"},
		{"declaration only", `<?xml version="1.1" encoding="UTF-8"?><r/>`, `<r/>`},
		{"leading whitespace", "\n\t<?xml version=\"1.1\"?><r/>", `<r/>`},
		{"no declaration", `<r/>`, `<r/>`},
		{"bom without declaration", "\xef\xbb\xbf<r/>", `<r/>`},
		{"other pi starting with xml", `<?xml-stylesheet href="s"?><r/>`, `<?xml-stylesheet href="s"?><r/>`},
		{"declaration missing terminator", `<?xml version="1.1"<r/>`, `<?xml version="1.1"<r/>`},
		{"bare target then terminator", `<?xml?><r/>`, `<r/>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, string(StripXMLDecl([]byte(c.in))))
		})
	}
}

func TestParseDOM_AcceptsXML11Declaration(t *testing.T) {
	root := mustDOM(t, "\xef\xbb\xbf<?xml version=\"1.1\" encoding=\"UTF-8\"?>\n<r name=\"x\"/>")
	assert.Equal(t, "x", root.Attr("name"))
}

func TestCheckAttrs(t *testing.T) {
	root := mustDOM(t, `<r a="1" b="2"/>`)
	require.NoError(t, CheckAttrs(root, "a", "b", "c"))

	err := CheckAttrs(root, "a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `<r>: unknown attribute "b"`)

	bare := mustDOM(t, `<r/>`)
	assert.NoError(t, CheckAttrs(bare))
}
