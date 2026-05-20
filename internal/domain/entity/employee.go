package entity

import "time"

// Employee mirrors the AD attributes we sync and store locally.
type Employee struct {
	ID          string    // sAMAccountName used as stable identifier
	DisplayName string    // displayName
	Mail        string    // mail
	Phone       string    // telephoneNumber
	Office      string    // physicalDeliveryOfficeName
	Groups      []string  // memberOf (distinguished names)
	SyncedAt    time.Time // last successful sync timestamp
}

// HasGroup reports whether the employee is a member of the given group DN.
func (e *Employee) HasGroup(dn string) bool {
	for _, g := range e.Groups {
		if g == dn {
			return true
		}
	}
	return false
}
