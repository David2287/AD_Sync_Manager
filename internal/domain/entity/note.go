package entity

import "time"

// Note is a Markdown correction note attached to an employee record.
type Note struct {
	ID         string
	EmployeeID string // references Employee.ID
	Content    string // raw Markdown source
	ParsedHTML string // rendered, sanitized HTML (cached)
	AuthorID   string // session UserID of the creator
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
