// Package apidsl is the lexical layer of a declarative XML API language.
//
// It holds three things and no vocabulary of its own:
//
//   - an order-preserving XML DOM (ParseDOM, Node), which keeps mixed content
//     in document order so a placeholder inside text survives the parse;
//   - a compiler that turns the placeholder elements <value>, <if> and <for>
//     into Go text/template source (CompileContent);
//   - the renderer that executes that source (RenderString, Renderer).
//
// A consumer supplies the structural vocabulary. It walks the DOM, names its
// own elements and attributes, and calls CompileContent on the content it
// treats as a template. A consumer that needs template functions of its own
// builds a Renderer with them.
package apidsl
