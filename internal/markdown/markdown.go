// Package markdown renders user-supplied Markdown to sanitized HTML and
// extracts @mentions. It has no database access.
package markdown

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
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
		goldmark.WithParserOptions(
			parser.WithASTTransformers(util.Prioritized(linkRelTransformer{}, 100)),
		),
	)

	// The UGC policy is too strict for our links, which carry a rel attribute
	// added by the renderer.
	sanitizer = newSanitizer()
)

func newSanitizer() *bluemonday.Policy {
	policy := bluemonday.UGCPolicy()
	policy.AllowAttrs("rel").Matching(bluemonday.SpaceSeparatedTokens).OnElements("a")
	return policy
}

// Render converts Markdown source to HTML and sanitizes the result. Links are
// given rel="nofollow noopener". The returned HTML is safe to insert into a
// page without further escaping.
func Render(source string) (template.HTML, error) {
	var buf bytes.Buffer
	if err := converter.Convert([]byte(source), &buf); err != nil {
		return "", err
	}

	return template.HTML(sanitizer.SanitizeBytes(buf.Bytes())), nil
}

// Mentions returns the distinct usernames mentioned as @username in source, in
// the order they first appear. Names are lowercased. Mentions inside code spans
// and code blocks are ignored, as are email-like "a@b" sequences, and names
// shorter than 3 or longer than 20 characters.
func Mentions(source string) []string {
	reader := text.NewReader([]byte(source))
	doc := goldmark.DefaultParser().Parse(reader)

	var mentions []string
	seen := make(map[string]struct{})

	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node.Kind() {
		case ast.KindCodeSpan, ast.KindCodeBlock, ast.KindFencedCodeBlock:
			return ast.WalkSkipChildren, nil
		case ast.KindText:
			// Goldmark splits text at characters that may start emphasis, such
			// as the underscore in "@user_name". Process the whole run of
			// adjacent text nodes so such mentions are not cut in half, but do
			// not merge across a soft or hard line break.
			if previous, ok := node.PreviousSibling().(*ast.Text); ok &&
				!previous.SoftLineBreak() && !previous.HardLineBreak() {
				return ast.WalkContinue, nil
			}

			var text []byte
			for current := node; current != nil && current.Kind() == ast.KindText; current = current.NextSibling() {
				currentText := current.(*ast.Text)
				text = append(text, currentText.Text(reader.Source())...)
				if currentText.SoftLineBreak() || currentText.HardLineBreak() {
					break
				}
			}

			for _, name := range namesInText(text) {
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				mentions = append(mentions, name)
			}
		}

		return ast.WalkContinue, nil
	})

	return mentions
}

// namesInText returns the mentions in a single run of text.
func namesInText(text []byte) []string {
	var names []string

	for i := 0; i < len(text); i++ {
		if text[i] != '@' {
			continue
		}
		// A username character before the @ makes this an email-like sequence
		// such as "a@b", not a mention.
		if i > 0 && isUsernameByte(text[i-1]) {
			continue
		}

		start := i + 1
		end := start
		for end < len(text) && isUsernameByte(text[end]) {
			end++
		}

		length := end - start
		if length >= minUsernameLength && length <= maxUsernameLength {
			names = append(names, strings.ToLower(string(text[start:end])))
		}
		i = end - 1
	}

	return names
}

func isUsernameByte(b byte) bool {
	return b >= 'a' && b <= 'z' ||
		b >= 'A' && b <= 'Z' ||
		b >= '0' && b <= '9' ||
		b == '_' || b == '-'
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
