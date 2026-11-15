// Package markdown renders user-supplied Markdown to sanitized HTML and
// extracts @mentions. It has no database access.
package markdown

import (
	"bytes"
	"html/template"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Username rules from the PRD. Usernames are case-insensitive and consist of
// ASCII letters, digits, underscores and hyphens, 3 to 20 characters long.
const (
	minUsernameLength = 3
	maxUsernameLength = 20
)

var (
	converter = goldmark.New(
		goldmark.WithExtensions(
			// GitHub Flavored Markdown: tables, strikethrough, linkify and
			// task lists.
			extension.GFM,
			mentionExtension{},
		),
		goldmark.WithParserOptions(
			parser.WithASTTransformers(util.Prioritized(linkRelTransformer{}, 100)),
		),
		goldmark.WithRendererOptions(
			// A single newline is a line break, as in GitHub comments.
			html.WithHardWraps(),
			// Tables emit align="left" rather than style="text-align: left";
			// the sanitizer drops style but keeps align.
			extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute),
		),
	)

	// The UGC policy is too strict for our links, which carry a rel attribute
	// added by the renderer, for mention spans, and for GFM task list
	// checkboxes.
	sanitizer = newSanitizer()
)

func newSanitizer() *bluemonday.Policy {
	policy := bluemonday.UGCPolicy()
	policy.AllowAttrs("rel").Matching(bluemonday.SpaceSeparatedTokens).OnElements("a")
	policy.AllowAttrs("class").Matching(regexp.MustCompile(`^mention$`)).OnElements("span")

	// Task list items render as <input type="checkbox" disabled checked>.
	// Allow nothing else on an input, so user HTML cannot turn it into another
	// kind of form field or give it a src. The policy never enables styles, so
	// a style attribute is dropped everywhere.
	policy.AllowAttrs("type").Matching(regexp.MustCompile(`(?i)^checkbox$`)).OnElements("input")
	policy.AllowAttrs("disabled").Matching(regexp.MustCompile(`(?i)^(|disabled)$`)).OnElements("input")
	policy.AllowAttrs("checked").Matching(regexp.MustCompile(`(?i)^(|checked)$`)).OnElements("input")

	return policy
}

// Render converts GitHub Flavored Markdown source to HTML and sanitizes the
// result. A single newline becomes a line break, as in GitHub comments. Links
// are given rel="nofollow noopener". The returned HTML is safe to insert into a
// page without further escaping.
func Render(source string) (template.HTML, error) {
	var buf bytes.Buffer
	if err := converter.Convert([]byte(source), &buf); err != nil {
		return "", err
	}

	return template.HTML(sanitizer.SanitizeBytes(buf.Bytes())), nil
}

// Mentions returns the distinct usernames mentioned as @username in source, in
// the order they first appear. Names are lowercased. It parses with the same
// parser as Render, so the names returned are exactly the ones rendered as
// mentions: mentions inside code are ignored, as are email-like sequences such
// as "a@b", URLs, and names shorter than 3 or longer than 20 characters.
func Mentions(source string) []string {
	reader := text.NewReader([]byte(source))
	doc := converter.Parser().Parse(reader)

	var mentions []string
	seen := make(map[string]struct{})

	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		mention, ok := node.(*mentionNode)
		if !ok {
			return ast.WalkContinue, nil
		}

		name := strings.ToLower(mention.Name)
		if _, ok := seen[name]; ok {
			return ast.WalkContinue, nil
		}
		seen[name] = struct{}{}
		mentions = append(mentions, name)

		return ast.WalkContinue, nil
	})

	return mentions
}

func isUsernameByte(b byte) bool {
	return b >= 'a' && b <= 'z' ||
		b >= 'A' && b <= 'Z' ||
		b >= '0' && b <= '9' ||
		b == '_' || b == '-'
}

func isUsernameRune(r rune) bool {
	return r < 0x80 && isUsernameByte(byte(r))
}

// mentionNode is an inline @username. The renderer wraps it in a span so the
// page can highlight it without making it a link.
type mentionNode struct {
	ast.BaseInline

	Name string
}

var mentionKind = ast.NewNodeKind("Mention")

func (n *mentionNode) Kind() ast.NodeKind {
	return mentionKind
}

func (n *mentionNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Name": n.Name}, nil)
}

// mentionParser turns @username into a mentionNode. Mentions reports these
// nodes, so notifications and highlighting always agree. Inline parsers are not
// run inside code spans or code blocks, so mentions there are ignored, and the
// preceding-character check keeps email-like "a@b" sequences out.
type mentionParser struct{}

func (mentionParser) Trigger() []byte {
	return []byte{'@'}
}

func (mentionParser) Parse(_ ast.Node, reader text.Reader, _ parser.Context) ast.Node {
	line, _ := reader.PeekLine()
	if len(line) == 0 || line[0] != '@' {
		return nil
	}
	if isUsernameRune(reader.PrecendingCharacter()) {
		return nil
	}

	end := 1
	for end < len(line) && isUsernameByte(line[end]) {
		end++
	}
	length := end - 1
	if length < minUsernameLength || length > maxUsernameLength {
		return nil
	}

	reader.Advance(end)
	return &mentionNode{Name: string(line[1:end])}
}

type mentionRenderer struct{}

func (mentionRenderer) RegisterFuncs(register renderer.NodeRendererFuncRegisterer) {
	register.Register(mentionKind, renderMention)
}

func renderMention(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	n := node.(*mentionNode)
	_, _ = w.WriteString(`<span class="mention">@`)
	_, _ = w.Write(util.EscapeHTML([]byte(n.Name)))
	_, _ = w.WriteString(`</span>`)
	return ast.WalkContinue, nil
}

type mentionExtension struct{}

func (mentionExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(util.Prioritized(mentionParser{}, 600)))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(mentionRenderer{}, 600)))
}

// linkRelTransformer adds rel="nofollow noopener" to every link before the HTML
// renderer runs.
type linkRelTransformer struct{}

func (linkRelTransformer) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := node.(type) {
		case *ast.Link:
			node.SetAttributeString("rel", []byte("nofollow noopener"))
		case *ast.AutoLink:
			node.SetAttributeString("rel", []byte("nofollow noopener"))
		}

		return ast.WalkContinue, nil
	})
}
