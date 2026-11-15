package markdown

import (
	"strings"
	"testing"
)

func TestRenderHighlightsMentions(t *testing.T) {
	got, err := Render("Hello @alice, not a@b, `@bob` or\n\n```\n@carol\n```")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	html := string(got)
	want := `<span class="mention">@alice</span>`
	if !strings.Contains(html, want) {
		t.Errorf("rendered HTML = %q, want it to contain %q", html, want)
	}
	for _, unwanted := range []string{
		`<span class="mention">@bob</span>`,
		`<span class="mention">@carol</span>`,
		`<span class="mention">@b</span>`,
	} {
		if strings.Contains(html, unwanted) {
			t.Errorf("rendered HTML highlights %q: %s", unwanted, html)
		}
	}
}

func TestRenderGFM(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{
			name:   "table alignment is an attribute",
			source: "| a | b |\n| :- | -: |\n| c | d |",
			want: []string{
				`<th align="left">a</th>`,
				`<th align="right">b</th>`,
				`<td align="left">c</td>`,
				`<td align="right">d</td>`,
			},
		},
		{
			name:   "strikethrough",
			source: "~~gone~~",
			want:   []string{"<del>gone</del>"},
		},
		{
			name:   "bare URL is linkified",
			source: "see https://example.com/path",
			want: []string{
				`<a href="https://example.com/path" rel="nofollow noopener">https://example.com/path</a>`,
			},
		},
		{
			name:   "URL with a mention-like path stays one link",
			source: "see https://example.com/@alice",
			want: []string{
				`<a href="https://example.com/@alice" rel="nofollow noopener">https://example.com/@alice</a>`,
			},
		},
		{
			name:   "task list",
			source: "- [ ] todo\n- [x] done",
			want: []string{
				`<input disabled="" type="checkbox"> todo`,
				`<input checked="" disabled="" type="checkbox"> done`,
			},
		},
		{
			name:   "single newline is a line break",
			source: "one\ntwo",
			want:   []string{"<p>one<br>\ntwo</p>"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Render(test.source)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}

			for _, want := range test.want {
				if !strings.Contains(string(got), want) {
					t.Errorf("Render(%q) = %q, want it to contain %q", test.source, got, want)
				}
			}
			if strings.Contains(string(got), "style=") {
				t.Errorf("Render(%q) = %q, want no style attribute", test.source, got)
			}
		})
	}
}

func TestSanitizerAllowsOnlyMentionClass(t *testing.T) {
	html := sanitizer.Sanitize(`<span class="mention">@alice</span><span class="other">text</span>`)

	if !strings.Contains(html, `<span class="mention">@alice</span>`) {
		t.Errorf("sanitized HTML = %q, want mention span", html)
	}
	if strings.Contains(html, `class="other"`) {
		t.Errorf("sanitized HTML kept a non-mention class: %q", html)
	}
}

func TestSanitizerAllowsOnlyCheckboxAttributesOnInput(t *testing.T) {
	html := sanitizer.Sanitize(`<input type="checkbox" checked disabled>` +
		`<input type="checkbox">` +
		`<input type="text">` +
		`<input>` +
		`<input type="checkbox" checked disabled name="x" value="y" src="https://example.com/x" style="color:red" onclick="alert(1)">`)

	// The bare <input>, and the one whose only attribute (type=text) is not
	// allowed, are dropped. The rest keep only allowed attributes.
	inputs := strings.Count(html, "<input")
	if inputs != 3 {
		t.Errorf("sanitized HTML keeps %d inputs, want 3: %q", inputs, html)
	}
	if got := strings.Count(html, `type="checkbox"`); got != inputs {
		t.Errorf("sanitized HTML has %d checkbox types for %d inputs: %q", got, inputs, html)
	}
	for _, wanted := range []string{`checked=""`, `disabled=""`} {
		if !strings.Contains(html, wanted) {
			t.Errorf("sanitized HTML = %q, want it to contain %q", html, wanted)
		}
	}
	for _, unwanted := range []string{`type="text"`, "name=", "value=", "src=", "style=", "onclick="} {
		if strings.Contains(html, unwanted) {
			t.Errorf("sanitized HTML = %q, want no %q", html, unwanted)
		}
	}
}

func TestSanitizerStripsStyle(t *testing.T) {
	html := sanitizer.Sanitize(`<span class="mention" style="color:red">@alice</span>` +
		`<td align="left" style="color:red">cell</td>`)

	if !strings.Contains(html, `<span class="mention">@alice</span>`) {
		t.Errorf("sanitized HTML = %q, want mention span", html)
	}
	if strings.Contains(html, "style") {
		t.Errorf("sanitized HTML kept a style attribute: %q", html)
	}
}

func TestRenderSanitizesHTML(t *testing.T) {
	source := `<script>alert("xss")</script>

<img src="x" onerror="alert('xss')">

[click](javascript:alert('xss'))
`

	got, err := Render(source)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	for _, unwanted := range []string{"script", "onerror", "javascript", "<img", "alert"} {
		if strings.Contains(strings.ToLower(string(got)), unwanted) {
			t.Errorf("rendered HTML contains %q: %s", unwanted, got)
		}
	}
}

func TestRenderAddsNoFollowNoOpener(t *testing.T) {
	got, err := Render("[example](https://example.com/)")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	want := `<a href="https://example.com/" rel="nofollow noopener">example</a>`
	if !strings.Contains(string(got), want) {
		t.Errorf("rendered HTML = %q, want it to contain %q", got, want)
	}
}

func TestRenderAutolink(t *testing.T) {
	got, err := Render("<https://example.com/>")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	want := `<a href="https://example.com/" rel="nofollow noopener">https://example.com/</a>`
	if !strings.Contains(string(got), want) {
		t.Errorf("rendered HTML = %q, want it to contain %q", got, want)
	}
}

func TestMentions(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{
			name:   "plain mentions",
			source: "Hello @alice, ask @Bob about it.",
			want:   []string{"alice", "bob"},
		},
		{
			name:   "deduplicated and lowercased",
			source: "@alice @ALICE @Alice",
			want:   []string{"alice"},
		},
		{
			name:   "punctuation around mentions",
			source: "(@alice), [@bob]! @carol.",
			want:   []string{"alice", "bob", "carol"},
		},
		{
			name:   "code span is ignored",
			source: "`@alice` and @bob",
			want:   []string{"bob"},
		},
		{
			name:   "fenced code block is ignored",
			source: "```\n@alice\n```\n\n@bob",
			want:   []string{"bob"},
		},
		{
			name:   "indented code block is ignored",
			source: "    @alice\n\n@bob",
			want:   []string{"bob"},
		},
		{
			name:   "email-like sequence is not a mention",
			source: "write to a@b or a@example.com, not @carol",
			want:   []string{"carol"},
		},
		{
			name:   "email address is not a mention",
			source: "write to a@b.com and @carol",
			want:   []string{"carol"},
		},
		{
			name:   "linkified URL is not a mention",
			source: "see https://example.com/@alice and @bob",
			want:   []string{"bob"},
		},
		{
			name:   "mentions in a table",
			source: "| who |\n| --- |\n| @alice |\n| @bob |",
			want:   []string{"alice", "bob"},
		},
		{
			name:   "too short",
			source: "@ab @a",
			want:   nil,
		},
		{
			name:   "too long",
			source: "@abcdefghijklmnopqrstu",
			want:   nil,
		},
		{
			name:   "maximum length",
			source: "@abcdefghijklmnopqrst",
			want:   []string{"abcdefghijklmnopqrst"},
		},
		{
			name:   "username punctuation",
			source: "@user_name and @user-name",
			want:   []string{"user_name", "user-name"},
		},
		{
			name:   "soft line break separates mentions",
			source: "@alice\n@bob",
			want:   []string{"alice", "bob"},
		},
		{
			name:   "mention split by soft line break",
			source: "@ali\nce",
			want:   []string{"ali"},
		},
		{
			name:   "hard line break separates mentions",
			source: "@alice  \n@bob",
			want:   []string{"alice", "bob"},
		},
		{
			name:   "mention split by hard line break",
			source: "@ali\\\nce",
			want:   []string{"ali"},
		},
		{
			name:   "no mentions",
			source: "nothing to see here",
			want:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Mentions(test.source)
			if len(got) != len(test.want) {
				t.Fatalf("Mentions(%q) = %v, want %v", test.source, got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Errorf("Mentions(%q) = %v, want %v", test.source, got, test.want)
					break
				}
			}
		})
	}
}
