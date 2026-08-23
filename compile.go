package apidsl

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// IsPlaceholder reports whether an element name belongs to the placeholder
// language. A consumer walks an element's children with it, to tell a
// placeholder from one of its own structural elements. Every other element
// name is structural, and belongs to the consumer's own vocabulary.
func IsPlaceholder(name string) bool {
	switch name {
	case "value", "if", "for", "else":
		return true
	}
	return false
}

// CompileContent compiles an element's mixed content (text plus
// <value>/<if>/<for>) into one Go template string.
func CompileContent(n *Node) (string, error) {
	return compileItems(n.content)
}

// CompileTextElem rejects every attribute on a content-only element, then
// compiles its mixed content.
func CompileTextElem(n *Node) (string, error) {
	if err := CheckAttrs(n); err != nil {
		return "", err
	}
	return CompileContent(n)
}

// TextOf returns the concatenated plain text of an element, and rejects a
// child element. Use it where the content must be literal, such as a
// description or a field path.
func TextOf(n *Node) (string, error) {
	var b strings.Builder
	for _, it := range n.content {
		if it.Elem != nil {
			return "", fmt.Errorf("<%s>: unexpected child element <%s>", n.name, it.Elem.name)
		}
		b.WriteString(cleanText(it.Text))
	}
	return strings.TrimSpace(b.String()), nil
}

// DotPath turns a context path ("var.base_url") into a template expression
// that reads it. A consumer calls it to compile its own path-valued attribute,
// such as a `when=` predicate, the same way the placeholders compile.
//
// An all-identifier path uses the readable field form (".var.base_url"), which
// is graceful on a missing key under missingkey=zero. A path with a
// non-identifier segment, such as the kebab-case "flag.dry-run", falls back to
// a parenthesized index expression. That form accepts an arbitrary key and
// works on map[string]any and map[string]string alike. Either form is safe
// bare after a function name, and inside parentheses.
func DotPath(path string) string {
	path = strings.TrimSpace(path)
	segs := strings.Split(path, ".")
	allIdent := true
	for _, s := range segs {
		if !isTemplateIdent(s) {
			allIdent = false
			break
		}
	}
	if allIdent {
		return "." + path
	}
	var b strings.Builder
	b.WriteString("(index .")
	for _, s := range segs {
		b.WriteByte(' ')
		b.WriteString(strconv.Quote(s))
	}
	b.WriteString(")")
	return b.String()
}

func compileItems(items []Item) (string, error) {
	var b strings.Builder
	for _, it := range items {
		if it.Elem == nil {
			b.WriteString(cleanText(it.Text))
			continue
		}
		var (
			s   string
			err error
		)
		switch it.Elem.name {
		case "value":
			s, err = compileValue(it.Elem)
		case "if":
			s, err = compileIf(it.Elem)
		case "for":
			s, err = compileFor(it.Elem)
		default:
			return "", fmt.Errorf("unexpected element <%s> in text content", it.Elem.name)
		}
		if err != nil {
			return "", err
		}
		b.WriteString(s)
	}
	return b.String(), nil
}

// cleanText normalizes a text chunk. A whitespace-only chunk that spans a line
// break is structural indentation, and it goes. Inline whitespace stays, such
// as the space in "Bearer ". Multi-line content loses its common leading tabs.
func cleanText(s string) string {
	if strings.TrimSpace(s) == "" {
		if strings.ContainsAny(s, "\n\r") {
			return ""
		}
		return s
	}
	if strings.Contains(s, "\n") {
		return dedentTabs(s)
	}
	return s
}

// dedentTabs removes the common leading-tab indentation from a multi-line
// block, and trims a leading and a trailing blank line. Structural indentation
// is tabs, so a leading space is content and it never counts.
func dedentTabs(s string) string {
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	min := -1
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		n := 0
		for n < len(ln) && ln[n] == '\t' {
			n++
		}
		if min < 0 || n < min {
			min = n
		}
	}
	if min > 0 {
		for i, ln := range lines {
			if len(ln) >= min {
				lines[i] = ln[min:]
			}
		}
	}
	return strings.Join(lines, "\n")
}

// isTemplateIdent reports whether s is a valid Go template field identifier.
func isTemplateIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r == '_' || unicode.IsLetter(r) {
			continue
		}
		if i > 0 && unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return true
}

func compileValue(n *Node) (string, error) {
	if err := CheckAttrs(n, "name", "expr", "default", "as"); err != nil {
		return "", err
	}
	name, expr := n.Attr("name"), n.Attr("expr")
	def, as := n.Attr("default"), n.Attr("as")
	if (name == "") == (expr == "") {
		return "", fmt.Errorf("<value>: exactly one of name= or expr= is required")
	}
	if expr != "" {
		if def != "" || as != "" {
			return "", fmt.Errorf("<value expr=>: cannot combine with default= or as=")
		}
		return expr, nil
	}
	core := DotPath(name)
	if def != "" {
		core = core + " | default " + strconv.Quote(def)
	}
	if as != "" {
		core = as + " (" + core + ")"
	}
	return "{{ " + core + " }}", nil
}

func compileIf(n *Node) (string, error) {
	if err := CheckAttrs(n, "test", "eq"); err != nil {
		return "", err
	}
	test := n.Attr("test")
	if test == "" {
		return "", fmt.Errorf("<if>: test= is required")
	}
	var thenItems, elseItems []Item
	cur := &thenItems
	seenElse := false
	for _, it := range n.content {
		if it.Elem != nil && it.Elem.name == "else" {
			if seenElse {
				return "", fmt.Errorf("<if>: at most one <else/> is allowed")
			}
			if err := CheckAttrs(it.Elem); err != nil {
				return "", err
			}
			seenElse = true
			cur = &elseItems
			continue
		}
		*cur = append(*cur, it)
	}
	thenStr, err := compileItems(thenItems)
	if err != nil {
		return "", err
	}
	elseStr, err := compileItems(elseItems)
	if err != nil {
		return "", err
	}
	var cond string
	if n.HasAttr("eq") {
		cond = fmt.Sprintf("eq (printf \"%%v\" %s) %s", DotPath(test), strconv.Quote(n.Attr("eq")))
	} else {
		cond = "truthy " + DotPath(test)
	}
	if seenElse {
		return fmt.Sprintf("{{ if %s }}%s{{ else }}%s{{ end }}", cond, thenStr, elseStr), nil
	}
	return fmt.Sprintf("{{ if %s }}%s{{ end }}", cond, thenStr), nil
}

// compileFor compiles <for each="path">...</for> to a range that rebinds "."
// to each element. A nested <value name="field"/> then reads the element's
// field, and <value expr="{{ . }}"/> reads a scalar element.
func compileFor(n *Node) (string, error) {
	if err := CheckAttrs(n, "each"); err != nil {
		return "", err
	}
	each := n.Attr("each")
	if each == "" {
		return "", fmt.Errorf("<for>: each= is required")
	}
	body, err := compileItems(n.content)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("{{ range %s }}%s{{ end }}", DotPath(each), body), nil
}
