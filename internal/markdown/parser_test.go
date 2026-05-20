package markdown

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validDoc = `# Employee Data Corrections

## Error 1: Wrong phone number
* **Employee:** ` + "`" + `CN=John Doe,OU=Employees,DC=company,DC=com` + "`" + `
* **Attribute:** ` + "`" + `telephoneNumber` + "`" + `
* **New value:** ` + "`" + `+7 999 123 45 67` + "`" + `

## Error 2: Missing office
* **Employee:** ` + "`" + `CN=Jane Smith,OU=Employees,DC=company,DC=com` + "`" + `
* **Attribute:** ` + "`" + `physicalDeliveryOfficeName` + "`" + `
* **New value:** ` + "`" + `B-203` + "`" + `
`

func TestParseMarkdown_Valid(t *testing.T) {
	ops, err := ParseMarkdown(validDoc)
	require.NoError(t, err)
	require.Len(t, ops, 2)

	assert.Equal(t, "CN=John Doe,OU=Employees,DC=company,DC=com", ops[0].DN)
	assert.Equal(t, "telephoneNumber", ops[0].Attribute)
	assert.Equal(t, "+7 999 123 45 67", ops[0].NewValue)

	assert.Equal(t, "CN=Jane Smith,OU=Employees,DC=company,DC=com", ops[1].DN)
	assert.Equal(t, "physicalDeliveryOfficeName", ops[1].Attribute)
	assert.Equal(t, "B-203", ops[1].NewValue)
}

func TestParseMarkdown_MissingTitle(t *testing.T) {
	doc := `## Error 1: Wrong phone
* **Employee:** ` + "`" + `CN=X,DC=y,DC=com` + "`" + `
* **Attribute:** ` + "`" + `telephoneNumber` + "`" + `
* **New value:** ` + "`" + `123` + "`" + `
`
	_, err := ParseMarkdown(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "# Employee Data Corrections")
}

func TestParseMarkdown_EmptyDocument(t *testing.T) {
	_, err := ParseMarkdown("")
	require.Error(t, err)
}

func TestParseMarkdown_MissingBackticks(t *testing.T) {
	doc := `# Employee Data Corrections

## Error 1: No backticks
* **Employee:** CN=Alice,OU=Employees,DC=company,DC=com
* **Attribute:** telephoneNumber
* **New value:** +7 000 000 00 00
`
	ops, err := ParseMarkdown(doc)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, "CN=Alice,OU=Employees,DC=company,DC=com", ops[0].DN)
	assert.Equal(t, "telephoneNumber", ops[0].Attribute)
	assert.Equal(t, "+7 000 000 00 00", ops[0].NewValue)
}

func TestParseMarkdown_ExtraSpaces(t *testing.T) {
	doc := `# Employee Data Corrections

## Error 1: Extra spaces
*  **Employee:**   ` + "`" + `CN=Bob,DC=company,DC=com` + "`" + `
*  **Attribute:**   telephoneNumber
*  **New value:**   hello world
`
	ops, err := ParseMarkdown(doc)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, "CN=Bob,DC=company,DC=com", ops[0].DN)
	assert.Equal(t, "telephoneNumber", ops[0].Attribute)
	assert.Equal(t, "hello world", ops[0].NewValue)
}

func TestParseMarkdown_CaseInsensitiveFieldLabels(t *testing.T) {
	doc := `# Employee Data Corrections

## Error 1: Upper case labels
* **EMPLOYEE:** CN=X,DC=y,DC=com
* **ATTRIBUTE:** mail
* **NEW VALUE:** x@example.com
`
	ops, err := ParseMarkdown(doc)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, "CN=X,DC=y,DC=com", ops[0].DN)
	assert.Equal(t, "mail", ops[0].Attribute)
	assert.Equal(t, "x@example.com", ops[0].NewValue)
}

func TestParseMarkdown_SkipsMalformedBlock(t *testing.T) {
	doc := `# Employee Data Corrections

## Error 1: Missing new value
* **Employee:** CN=X,DC=y,DC=com
* **Attribute:** telephoneNumber

## Error 2: Complete block
* **Employee:** CN=Good,DC=y,DC=com
* **Attribute:** mail
* **New value:** good@example.com
`
	ops, err := ParseMarkdown(doc)
	require.NoError(t, err)
	// Block 1 is skipped because New value is absent; block 2 is returned.
	require.Len(t, ops, 1)
	assert.Equal(t, "CN=Good,DC=y,DC=com", ops[0].DN)
}

func TestParseMarkdown_MultipleBlocks(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("# Employee Data Corrections\n\n")
	for i := 1; i <= 5; i++ {
		sb.WriteString("## Error " + string(rune('0'+i)) + ": test\n")
		sb.WriteString("* **Employee:** CN=User" + string(rune('0'+i)) + ",DC=c,DC=com\n")
		sb.WriteString("* **Attribute:** telephoneNumber\n")
		sb.WriteString("* **New value:** +7 000 000 000" + string(rune('0'+i)) + "\n\n")
	}
	ops, err := ParseMarkdown(sb.String())
	require.NoError(t, err)
	assert.Len(t, ops, 5)
}

func TestParseMarkdown_WindowsLineEndings(t *testing.T) {
	doc := "# Employee Data Corrections\r\n\r\n## Error 1: CRLF\r\n" +
		"* **Employee:** CN=X,DC=y,DC=com\r\n" +
		"* **Attribute:** telephoneNumber\r\n" +
		"* **New value:** +1 234 567 8900\r\n"

	ops, err := ParseMarkdown(doc)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, "+1 234 567 8900", ops[0].NewValue)
}

func TestParseMarkdown_AllowedAttributeNames(t *testing.T) {
	attrs := []string{"telephoneNumber", "physicalDeliveryOfficeName", "mail", "displayName"}
	for _, attr := range attrs {
		doc := "# Employee Data Corrections\n\n## Error 1: attr\n" +
			"* **Employee:** CN=X,DC=y,DC=com\n" +
			"* **Attribute:** " + attr + "\n" +
			"* **New value:** somevalue\n"

		ops, err := ParseMarkdown(doc)
		require.NoError(t, err, "attribute %q should parse without error", attr)
		require.Len(t, ops, 1)
		assert.Equal(t, attr, ops[0].Attribute)
	}
}

func TestParseMarkdown_OnlyTitleNoBlocks(t *testing.T) {
	ops, err := ParseMarkdown("# Employee Data Corrections\n")
	require.NoError(t, err)
	assert.Empty(t, ops)
}
