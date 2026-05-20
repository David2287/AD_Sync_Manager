package entity

import "time"

// Session represents an authenticated user's runtime context.
type Session struct {
	UserID    string    // sAMAccountName
	Username  string    // human-friendly display name
	Groups    []string  // AD group DNs at login time
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// IsAdmin reports whether the session includes the given admin group DN.
func (s *Session) IsAdmin(adminGroupDN string) bool {
	for _, g := range s.Groups {
		if g == adminGroupDN {
			return true
		}
	}
	return false
}

// Claims is the JWT payload structure.
type Claims struct {
	UserID   string   `json:"uid"`
	Username string   `json:"sub"`
	Groups   []string `json:"grp"`
}
