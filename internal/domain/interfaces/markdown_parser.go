package interfaces

// MarkdownParser is the port for converting raw note content to safe HTML.
// The production adapter uses github.com/yuin/goldmark with a sanitiser.
type MarkdownParser interface {
	// Parse converts markdown src to sanitized HTML.
	Parse(src []byte) ([]byte, error)

	// Validate checks syntax without rendering; returns a descriptive error
	// if the markdown would produce unsafe or structurally invalid output.
	Validate(src []byte) error
}
