// Package markdown renders user-supplied Markdown to sanitized HTML and
// extracts @mentions. It has no database access.
package markdown

import (
	"bytes"
	"html/template"
	"regexp"
	"strconv"
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
	nethtml "golang.org/x/net/html"
)

// Username rules from the PRD. Usernames are case-insensitive and consist of
// ASCII letters, digits, underscores and hyphens, 3 to 20 characters long.
const (
	minUsernameLength = 3
	maxUsernameLength = 20
)

var (
	// Both converters share every option except the thread-only heading id
	// transformer, so comments and thread bodies cannot drift.
	converter       = newConverter()
	threadConverter = newConverter(headingIDTransformer{})

	// The UGC policy is too strict for our links, which carry a rel attribute
	// added by the renderer, for mention spans, and for GFM task list
	// checkboxes.
	sanitizer = newSanitizer()
)

func newConverter(extra ...parser.ASTTransformer) goldmark.Markdown {
	transformers := []util.PrioritizedValue{
		util.Prioritized(linkRelTransformer{}, 100),
		// The page title is the only h1, so headings in every body start at
		// h2.
		util.Prioritized(headingShiftTransformer{}, 200),
	}
	for _, transformer := range extra {
		transformers = append(transformers, util.Prioritized(transformer, 300))
	}

	return goldmark.New(
		goldmark.WithExtensions(
			// GitHub Flavored Markdown: tables, strikethrough, linkify and
			// task lists.
			extension.GFM,
			mentionExtension{},
		),
		goldmark.WithParserOptions(
			parser.WithASTTransformers(transformers...),
		),
		goldmark.WithRendererOptions(
			// A single newline is a line break, as in GitHub comments.
			html.WithHardWraps(),
			// Tables emit align="left" rather than style="text-align: left";
			// the sanitizer drops style but keeps align.
			extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute),
		),
	)
}

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
// are given rel="nofollow noopener". Headings shift down one level, so a body
// starts at h2 below the page title. The returned HTML is safe to insert into a
// page without further escaping.
func Render(source string) (template.HTML, error) {
	rendered, err := render(source)
	if err != nil {
		return "", err
	}
	return template.HTML(rendered), nil
}

// RenderThread renders a thread body like Render and gives its headings ids
// for the table of contents. The id prefix keeps them clear of the page ids
// that already exist (comments, c<id>, t<id> and so on), and only ASCII
// characters are used so the sanitizer keeps them.
func RenderThread(source string) (template.HTML, error) {
	rendered, err := renderWith(threadConverter, source)
	if err != nil {
		return "", err
	}
	return template.HTML(rendered), nil
}

func render(source string) (string, error) {
	return renderWith(converter, source)
}

func renderWith(markdown goldmark.Markdown, source string) (string, error) {
	var buf bytes.Buffer
	if err := markdown.Convert([]byte(source), &buf); err != nil {
		return "", err
	}

	return string(sanitizer.SanitizeBytes(buf.Bytes())), nil
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

// headingShiftTransformer moves Markdown headings down one level. A body has no
// h1 of its own: the page title is the h1, and a body's first heading is an h2.
type headingShiftTransformer struct{}

func (headingShiftTransformer) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if heading, ok := node.(*ast.Heading); ok && heading.Level < 6 {
			heading.Level++
		}

		return ast.WalkContinue, nil
	})
}

// TOCEntry is one heading in a thread's table of contents. Children are the h3
// headings that follow an h2.
type TOCEntry struct {
	ID       string
	Text     string
	Children []TOCEntry
	// Current marks the entry the page starts on. app.js moves it as the reader
	// scrolls.
	Current bool
}

// TableOfContents returns the h2 and h3 headings that have an id, in document
// order, with h3 headings nested under the preceding h2. It reads stored
// body_html, so it reflects what the thread page actually shows.
func TableOfContents(bodyHTML string) []TOCEntry {
	nodes, err := nethtml.ParseFragment(strings.NewReader(bodyHTML), nil)
	if err != nil {
		return nil
	}

	var entries []TOCEntry
	top := -1
	topIsH2 := false

	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		if node.Type == nethtml.ElementNode && (node.Data == "h2" || node.Data == "h3") {
			id := ""
			for _, attr := range node.Attr {
				if attr.Key == "id" {
					id = attr.Val
					break
				}
			}
			text := strings.Join(strings.Fields(textContent(node)), " ")
			if id != "" && text != "" {
				entry := TOCEntry{ID: id, Text: text}
				if node.Data == "h2" {
					entries = append(entries, entry)
					top = len(entries) - 1
					topIsH2 = true
				} else if top >= 0 && topIsH2 {
					entries[top].Children = append(entries[top].Children, entry)
				} else {
					entries = append(entries, entry)
					top = len(entries) - 1
					topIsH2 = false
				}
			}
			return
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}

	for _, node := range nodes {
		walk(node)
	}
	if len(entries) > 0 {
		entries[0].Current = true
	}
	return entries
}

func textContent(node *nethtml.Node) string {
	if node.Type == nethtml.TextNode {
		return node.Data
	}

	var text strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == nethtml.ElementNode && (child.Data == "script" || child.Data == "style") {
			continue
		}
		text.WriteString(textContent(child))
	}
	return text.String()
}

// headingIDTransformer gives every heading an id derived from its text, unique
// within the body. It is only part of the thread body converter. The ids are
// ASCII, so the sanitizer keeps them, and the h- prefix keeps them clear of the
// page ids that already exist.
type headingIDTransformer struct{}

func (headingIDTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	source := reader.Source()
	used := make(map[string]struct{})
	section := 0

	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		heading, ok := node.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		section++
		id := headingID(headingSlug(headingText(heading, source)), section, used)
		heading.SetAttributeString("id", []byte(id))

		return ast.WalkContinue, nil
	})
}

// headingText collects the visible text of a heading for its slug. Mentions are
// inline nodes of our own, and autolinks keep their label rather than child
// text nodes, so both need explicit handling.
func headingText(heading *ast.Heading, source []byte) string {
	var text strings.Builder

	_ = ast.Walk(heading, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := node.(type) {
		case *ast.Text:
			text.Write(node.Segment.Value(source))
			if node.SoftLineBreak() || node.HardLineBreak() {
				text.WriteByte(' ')
			}
		case *ast.String:
			text.Write(node.Value)
		case *ast.AutoLink:
			text.Write(node.Label(source))
		case *mentionNode:
			text.WriteString(node.Name)
		}

		return ast.WalkContinue, nil
	})

	return text.String()
}

// headingID turns a slug into a unique id. An empty slug is numbered, so a
// heading that is only punctuation or non-ASCII text still gets an anchor.
func headingID(slug string, section int, used map[string]struct{}) string {
	base := "h-" + slug
	if slug == "" {
		base = "h-section-" + strconv.Itoa(section)
	}

	id := base
	for suffix := 2; ; suffix++ {
		if _, exists := used[id]; !exists {
			break
		}
		id = base + "-" + strconv.Itoa(suffix)
	}

	used[id] = struct{}{}
	return id
}

// headingSlug keeps ASCII letters and digits, turns runs of spaces, hyphens and
// underscores into one hyphen, and drops everything else. Ids must be ASCII to
// survive the sanitizer, so non-ASCII letters are dropped rather than escaped.
func headingSlug(text string) string {
	var slug strings.Builder
	separator := false

	for _, r := range strings.ToLower(text) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			if separator && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			separator = false
			slug.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			separator = true
		}
	}

	return slug.String()
}
