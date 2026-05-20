package auth

import "context"

// LDAPCred carries the user's LDAP bind DN and password within a single
// request context. Injected by RequireAuth middleware when AD_USE_USER_BIND=true;
// consumed by the employee repository to bind as the authenticated user rather
// than the service account.
type LDAPCred struct {
	DN       string // user's full Distinguished Name
	Password string // plaintext password — lives only for the duration of the request
}

type ldapCredKey struct{}

// ContextWithLDAPCred returns a new context carrying the given LDAPCred.
func ContextWithLDAPCred(ctx context.Context, cred LDAPCred) context.Context {
	return context.WithValue(ctx, ldapCredKey{}, cred)
}

// LDAPCredFromContext extracts the LDAPCred from a context.
// Returns the zero value and false when no credential was stored (e.g. the
// request was served without AD_USE_USER_BIND=true, or from a background task).
func LDAPCredFromContext(ctx context.Context) (LDAPCred, bool) {
	cred, ok := ctx.Value(ldapCredKey{}).(LDAPCred)
	return cred, ok && cred.DN != ""
}
