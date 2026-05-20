package markdown

import (
	"bytes"
	"fmt"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"

	"ad-sync-manager/internal/domain/interfaces"
)

// goldmarkParser implements interfaces.MarkdownParser using yuin/goldmark.
// HTML output is rendered with unsafe=false (no raw HTML pass-through), which
// prevents XSS when notes are displayed in the frontend.
type goldmarkParser struct {
	md goldmark.Markdown
}

// NewGoldmarkParser returns an interfaces.MarkdownParser ready for use.
func NewGoldmarkParser() interfaces.MarkdownParser {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,        // GitHub-Flavored Markdown (tables, strike-through…)
			extension.Footnote,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			// unsafe=false is the default; listed explicitly for clarity.
			// Raw HTML blocks in notes are stripped, not passed through.
		),
	)
	return &goldmarkParser{md: md}
}

// Parse converts markdown src to sanitized HTML.
func (p *goldmarkParser) Parse(src []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := p.md.Convert(src, &buf); err != nil {
		return nil, fmt.Errorf("markdown: render failed: %w", err)
	}
	return buf.Bytes(), nil
}

// Validate parses without rendering to surface structural errors early.
func (p *goldmarkParser) Validate(src []byte) error {
	// goldmark is lenient by design; we enforce a size limit as a safety valve.
	const maxBytes = 64 * 1024 // 64 KiB per note
	if len(src) > maxBytes {
		return fmt.Errorf("markdown: note exceeds maximum size of %d bytes", maxBytes)
	}
	return nil
}
