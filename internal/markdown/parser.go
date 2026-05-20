package markdown

import (
	"errors"
	"regexp"
	"strings"
)

// errorBlockRe matches the start of a "## Error N:" section header.
// The error number and the trailing description text are ignored; only the
// "## Error" prefix is significant.
var errorBlockRe = regexp.MustCompile(`(?i)^##\s+Error\s+`)

// ParseMarkdown parses a correction document and returns one MarkdownOperation
// per well-formed error block. Blocks that are missing any of the three required
// fields (Employee, Attribute, New value) are silently skipped.
//
// Returns a non-nil error when the document is structurally invalid (e.g. the
// required title is absent), which causes the HTTP handler to respond with 400.
//
// The parser is intentionally lenient:
//   - Backtick delimiters around values are optional.
//   - Leading/trailing whitespace around values is stripped.
//   - Field labels are matched case-insensitively.
//   - Blank lines between blocks are ignored.
func ParseMarkdown(input string) ([]MarkdownOperation, error) {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	lines := strings.Split(input, "\n")

	titleFound := false
	for _, l := range lines {
		if strings.TrimSpace(l) == "# Employee Data Corrections" {
			titleFound = true
			break
		}
	}
	if !titleFound {
		return nil, errors.New("document must begin with '# Employee Data Corrections'")
	}

	// Collect the lines belonging to each ## Error N: block.
	var blocks [][]string
	var cur []string
	inBlock := false

	for _, l := range lines {
		if errorBlockRe.MatchString(strings.TrimSpace(l)) {
			if inBlock {
				blocks = append(blocks, cur)
			}
			cur = nil
			inBlock = true
			continue
		}
		if inBlock {
			cur = append(cur, l)
		}
	}
	if inBlock {
		blocks = append(blocks, cur)
	}

	var ops []MarkdownOperation
	for _, b := range blocks {
		if op, ok := parseBlock(b); ok {
			ops = append(ops, op)
		}
	}
	return ops, nil
}

// parseBlock extracts a MarkdownOperation from the body lines of one error block.
// Returns (op, true) when all three required fields are present.
func parseBlock(lines []string) (MarkdownOperation, bool) {
	var op MarkdownOperation
	for _, l := range lines {
		if v, ok := extractField(l, "Employee"); ok {
			op.DN = v
		}
		if v, ok := extractField(l, "Attribute"); ok {
			op.Attribute = v
		}
		if v, ok := extractField(l, "New value"); ok {
			op.NewValue = v
		}
	}
	if op.DN == "" || op.Attribute == "" || op.NewValue == "" {
		return MarkdownOperation{}, false
	}
	return op, true
}

// extractField searches line for "**<label>:**" (case-insensitive) and returns
// the text that follows, stripped of surrounding whitespace and optional backtick
// delimiters.
func extractField(line, label string) (string, bool) {
	needle := "**" + strings.ToLower(label) + ":**"
	lower := strings.ToLower(line)
	idx := strings.Index(lower, needle)
	if idx == -1 {
		return "", false
	}
	rest := strings.TrimSpace(line[idx+len(needle):])
	rest = strings.Trim(rest, "`")
	rest = strings.TrimSpace(rest)
	return rest, true
}
