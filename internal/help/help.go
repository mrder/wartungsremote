// Package help renders docs/DASHBOARD_HELP.md into dashboard help pages
// (docs/API.md §19: GET /help/index, GET /help/:slug). The Markdown
// subset supported is deliberately small — exactly what DASHBOARD_HELP.md
// actually uses (headings, paragraphs, bold, lists, code) — rather than a
// general-purpose parser, and every text run is HTML-escaped before
// emission. Only a small fixed allowlist of tags is ever produced, so this
// IS the "HTML sanitizen" step (docs/TODO.md Phase 38), not a separate
// pass over untrusted output.
package help

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Section is one top-level (H2) topic, addressable by Slug.
type Section struct {
	Slug  string
	Title string
	HTML  string
}

// Load reads DASHBOARD_HELP.md from dir and splits it into sections at
// each top-level "## " heading. Content before the first H2 (the H1 title
// and intro line) is dropped from the section list but not an error —
// docs/DASHBOARD_HELP.md's own intro is about the file's purpose, not
// end-user help content.
func Load(dir string) ([]Section, error) {
	path := filepath.Join(dir, "DASHBOARD_HELP.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("help: read %s: %w", path, err)
	}
	return Parse(string(data))
}

var h2Pattern = regexp.MustCompile(`^## (.+)$`)

func Parse(markdown string) ([]Section, error) {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")

	var sections []Section
	var title string
	var body []string
	seen := map[string]int{}

	flush := func() {
		if title == "" {
			return
		}
		slug := slugify(title)
		if n := seen[slug]; n > 0 {
			slug = fmt.Sprintf("%s-%d", slug, n+1)
		}
		seen[slug]++
		sections = append(sections, Section{Slug: slug, Title: title, HTML: RenderMarkdown(strings.Join(body, "\n"))})
	}

	for _, line := range lines {
		if m := h2Pattern.FindStringSubmatch(line); m != nil {
			flush()
			title = strings.TrimSpace(m[1])
			body = nil
			continue
		}
		if title != "" {
			body = append(body, line)
		}
	}
	flush()

	return sections, nil
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// slugify produces a stable, URL-safe identifier from a heading. German
// umlauts/ß are transliterated rather than dropped so headings that differ
// only by them don't collide.
func slugify(title string) string {
	s := strings.ToLower(title)
	replacer := strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss")
	s = replacer.Replace(s)
	s = nonAlnum.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

var (
	h3Pattern      = regexp.MustCompile(`^### (.+)$`)
	ulPattern      = regexp.MustCompile(`^- (.+)$`)
	olPattern      = regexp.MustCompile(`^\d+\. (.+)$`)
	boldPattern    = regexp.MustCompile(`\*\*(.+?)\*\*`)
	inlineCodeExpr = regexp.MustCompile("`([^`]+)`")
)

// RenderMarkdown converts the small Markdown subset used throughout
// DASHBOARD_HELP.md into HTML built only from a fixed tag allowlist
// (h3, p, ul/li, ol/li, pre/code, strong, code). All source text passes
// through html.EscapeString before any tag is wrapped around it, so
// literal "<script>" or similar in the source renders as inert text,
// never as markup.
func RenderMarkdown(md string) string {
	lines := strings.Split(md, "\n")
	var out strings.Builder

	var listBuf []string
	listKind := "" // "ul" | "ol"
	flushList := func() {
		if listKind == "" {
			return
		}
		out.WriteString("<" + listKind + ">")
		for _, item := range listBuf {
			out.WriteString("<li>" + inlineMarkdown(item) + "</li>")
		}
		out.WriteString("</" + listKind + ">")
		listBuf = nil
		listKind = ""
	}

	inCode := false
	var codeBuf []string
	flushCode := func() {
		out.WriteString("<pre><code>" + html.EscapeString(strings.Join(codeBuf, "\n")) + "</code></pre>")
		codeBuf = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				flushCode()
				inCode = false
			} else {
				flushList()
				inCode = true
			}
			continue
		}
		if inCode {
			codeBuf = append(codeBuf, line)
			continue
		}

		if trimmed == "" {
			flushList()
			continue
		}
		if m := h3Pattern.FindStringSubmatch(trimmed); m != nil {
			flushList()
			out.WriteString("<h3>" + html.EscapeString(strings.TrimSpace(m[1])) + "</h3>")
			continue
		}
		if m := ulPattern.FindStringSubmatch(trimmed); m != nil {
			if listKind != "ul" {
				flushList()
				listKind = "ul"
			}
			listBuf = append(listBuf, m[1])
			continue
		}
		if m := olPattern.FindStringSubmatch(trimmed); m != nil {
			if listKind != "ol" {
				flushList()
				listKind = "ol"
			}
			listBuf = append(listBuf, m[1])
			continue
		}
		flushList()
		out.WriteString("<p>" + inlineMarkdown(trimmed) + "</p>")
	}
	flushList()
	if inCode {
		flushCode()
	}

	return out.String()
}

// inlineMarkdown escapes text first, then re-introduces exactly two safe
// inline constructs (bold, inline code) by operating on the already-escaped
// string — so escaping can never be bypassed by markup embedded in the
// source.
func inlineMarkdown(s string) string {
	escaped := html.EscapeString(s)
	escaped = boldPattern.ReplaceAllString(escaped, "<strong>$1</strong>")
	escaped = inlineCodeExpr.ReplaceAllString(escaped, "<code>$1</code>")
	return escaped
}
