package employee

// Employee is the REST API representation of an Active Directory user account.
// It is the Stage 3 view model returned by the employee endpoints.
// It is distinct from domain/entity.Employee, which is the SQLite-backed model
// used by the background sync subsystem.
type Employee struct {
	DN              string `json:"dn"`
	FullName        string `json:"fullName"`
	Email           string `json:"email"`
	Office          string `json:"office"`
	TelephoneNumber string `json:"telephoneNumber"`
}
