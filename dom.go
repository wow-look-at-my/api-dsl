package apidsl

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"slices"
)

// Node is a parsed XML element. Content preserves the original order of text
// chunks and child elements, so mixed content compiles faithfully.
type Node struct {
	name    string
	attrs   []Attr
	attrMap map[string]string
	content []Item
}

// Attr is one attribute of an element, in document order.
type Attr struct {
	Name  string
	Value string
}

// Item is one piece of an element's content. Exactly one of Text and Elem is
// set: Elem == nil means the item is a text chunk.
type Item struct {
	Text  string
	Elem  *Node
	CDATA bool
}

// IsText reports whether the item is a text chunk.
func (i Item) IsText() bool { return i.Elem == nil }

// Name returns the element's local name. A namespace prefix is dropped.
func (n *Node) Name() string { return n.name }

// Attr returns the attribute's value, or "" when the element does not carry it.
func (n *Node) Attr(name string) string { return n.attrMap[name] }

// HasAttr reports whether the element carries the attribute. Use it to tell an
// absent attribute from one set to the empty string.
func (n *Node) HasAttr(name string) bool { _, ok := n.attrMap[name]; return ok }

// Attrs returns the element's attributes in document order.
func (n *Node) Attrs() []Attr { return n.attrs }

// Content returns the element's mixed content in document order.
func (n *Node) Content() []Item { return n.content }

// Children returns the child elements. Text chunks are dropped.
func (n *Node) Children() []*Node {
	var out []*Node
	for _, it := range n.content {
		if it.Elem != nil {
			out = append(out, it.Elem)
		}
	}
	return out
}

// xsiNamespace is the XML Schema instance namespace. Its attributes.
const xsiNamespace = "http://www.w3.org/2001/XMLSchema-instance"

// ParseDOM tokenizes an XML document into an order-preserving DOM. Comments,
// processing instructions, and directives are dropped. A CDATA section
// arrives as a text chunk of its own, with Item.CDATA set. Namespace
// declarations and xsi: attributes are dropped, so CheckAttrs never sees
// them.
//
// The caller checks the root element's name. This package is the lexical
// layer under several consumers, and each names its own root element, so a
// root name here would put one consumer's vocabulary in a shared module. The
// error for a second top-level element therefore stays generic.
func ParseDOM(src []byte) (*Node, error) {
	body := StripXMLDecl(src)
	dec := xml.NewDecoder(bytes.NewReader(body))
	dec.Strict = true
	var root *Node
	var stack []*Node
	for {
		start := dec.InputOffset()
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &Node{name: t.Name.Local, attrMap: map[string]string{}}
			for _, a := range t.Attr {
				if a.Name.Local == "xmlns" || a.Name.Space == "xmlns" || a.Name.Space == xsiNamespace {
					continue
				}
				if _, dup := n.attrMap[a.Name.Local]; dup {
					return nil, fmt.Errorf("<%s>: duplicate attribute %q", n.name, a.Name.Local)
				}
				n.attrs = append(n.attrs, Attr{a.Name.Local, a.Value})
				n.attrMap[a.Name.Local] = a.Value
			}
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.content = append(p.content, Item{Elem: n})
			}
			stack = append(stack, n)
		case xml.EndElement:
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("multiple top-level elements; expected a single root element")
				}
				root = top
			}
		case xml.CharData:
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.content = append(p.content, Item{Text: string(t), CDATA: isCDATA(body, start)})
			}
		}
	}
	if root == nil {
		return nil, fmt.Errorf("no root element")
	}
	return root, nil
}

// isCDATA reports whether the token that starts at offset is a CDATA section.
func isCDATA(src []byte, offset int64) bool {
	return offset >= 0 && offset < int64(len(src)) && bytes.HasPrefix(src[offset:], []byte("<![CDATA["))
}

// StripXMLDecl removes a leading XML declaration (<?xml ... ?>) and a UTF-8
// BOM. Go's encoding/xml supports only XML 1.0 and errors on a version="1.1"
// declaration, but a shipped document declares 1.1 for the stricter external
// validator. The declaration carries nothing a consumer needs, so it goes
// before the decode.
func StripXMLDecl(src []byte) []byte {
	src = bytes.TrimPrefix(src, []byte("\xef\xbb\xbf")) // UTF-8 BOM
	trimmed := bytes.TrimLeft(src, " \t\r\n")
	if !bytes.HasPrefix(trimmed, []byte("<?xml")) {
		return src
	}
	rest := trimmed[len("<?xml"):]
	// Treat it as the declaration only when the next byte ends the target or
	// is whitespace. Another processing instruction can also start with "xml".
	if len(rest) > 0 && rest[0] != ' ' && rest[0] != '\t' && rest[0] != '\r' && rest[0] != '\n' && rest[0] != '?' {
		return src
	}
	i := bytes.Index(trimmed, []byte("?>"))
	if i < 0 {
		return src
	}
	return trimmed[i+2:]
}

// CheckAttrs rejects any attribute on n outside the allowed set. Call it on
// every element a consumer builds, so a typo fails the load instead of going
// unread.
func CheckAttrs(n *Node, allowed ...string) error {
	for _, a := range n.attrs {
		if !slices.Contains(allowed, a.Name) {
			return fmt.Errorf("<%s>: unknown attribute %q", n.name, a.Name)
		}
	}
	return nil
}
