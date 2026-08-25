package help

import (
	"strings"
	"testing"
)

func TestParseSplitsOnH2Sections(t *testing.T) {
	md := "# Title\n\nIntro paragraph, not a section.\n\n## First Topic\n\nBody one.\n\n## Second Topic\n\nBody two.\n"
	sections, err := Parse(md)
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
	if sections[0].Title != "First Topic" || sections[0].Slug != "first-topic" {
		t.Fatalf("unexpected first section: %+v", sections[0])
	}
	if sections[1].Title != "Second Topic" || sections[1].Slug != "second-topic" {
		t.Fatalf("unexpected second section: %+v", sections[1])
	}
	if !strings.Contains(sections[0].HTML, "Body one.") {
		t.Fatalf("expected body content in HTML, got %q", sections[0].HTML)
	}
}

func TestSlugifyHandlesGermanAndDuplicates(t *testing.T) {
	md := "## Gerät zeigt falsche/alte öffentliche IP\n\nBody.\n\n## Gerät zeigt falsche/alte öffentliche IP\n\nBody two.\n"
	sections, err := Parse(md)
	if err != nil {
		t.Fatal(err)
	}
	if sections[0].Slug != "geraet-zeigt-falsche-alte-oeffentliche-ip" {
		t.Fatalf("unexpected slug: %q", sections[0].Slug)
	}
	if sections[1].Slug == sections[0].Slug {
		t.Fatalf("expected duplicate heading to get a distinct slug, got %q twice", sections[0].Slug)
	}
}

func TestRenderMarkdownEscapesLiteralHTML(t *testing.T) {
	got := RenderMarkdown("<script>alert(1)</script> and **bold** and `code`")
	if strings.Contains(got, "<script>") {
		t.Fatalf("expected literal <script> to be escaped, got %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("expected escaped script tag in output, got %q", got)
	}
	if !strings.Contains(got, "<strong>bold</strong>") {
		t.Fatalf("expected bold markdown to render, got %q", got)
	}
	if !strings.Contains(got, "<code>code</code>") {
		t.Fatalf("expected inline code to render, got %q", got)
	}
}

func TestRenderMarkdownBoldCannotInjectMarkup(t *testing.T) {
	// A malicious-looking bold span must still come out escaped — bolding
	// happens on the already-escaped string, so this must never produce a
	// live <img> tag.
	got := RenderMarkdown("**<img src=x onerror=alert(1)>**")
	if strings.Contains(got, "<img") {
		t.Fatalf("expected embedded HTML inside bold markdown to be escaped, got %q", got)
	}
}

func TestRenderMarkdownLists(t *testing.T) {
	got := RenderMarkdown("- one\n- two\n\n1. first\n2. second\n")
	if !strings.Contains(got, "<ul><li>one</li><li>two</li></ul>") {
		t.Fatalf("expected unordered list, got %q", got)
	}
	if !strings.Contains(got, "<ol><li>first</li><li>second</li></ol>") {
		t.Fatalf("expected ordered list, got %q", got)
	}
}

func TestRenderMarkdownCodeFence(t *testing.T) {
	got := RenderMarkdown("```text\nsystemctl status foo\n```")
	if !strings.Contains(got, "<pre><code>systemctl status foo</code></pre>") {
		t.Fatalf("expected code fence rendered as pre/code, got %q", got)
	}
}

func TestLoadRealDashboardHelp(t *testing.T) {
	sections, err := Load("../../docs")
	if err != nil {
		t.Fatalf("expected to load docs/DASHBOARD_HELP.md, got error: %v", err)
	}
	if len(sections) < 10 {
		t.Fatalf("expected the real help doc to yield many sections, got %d", len(sections))
	}
	for _, s := range sections {
		if s.Slug == "" || s.Title == "" {
			t.Fatalf("section missing slug/title: %+v", s)
		}
	}
}
