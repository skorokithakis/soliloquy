package markdown

import (
	"strings"
	"testing"
)

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
