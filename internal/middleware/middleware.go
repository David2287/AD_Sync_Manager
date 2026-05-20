package middleware

import "ad-sync-manager/internal/domain/interfaces"

// Bundle groups all middleware constructors so handlers.NewRouter receives
// a single typed dependency rather than many loose functions.
type Bundle struct {
	log          interfaces.Logger
	adminGroupDN string
	useUserBind  bool
}

// New constructs a middleware Bundle.
// sessionSvc is accepted as nil for backwards-compatibility and is ignored.
// Set useUserBind=true when AD_USE_USER_BIND is enabled; RequireAuth will then
// inject per-user LDAP credentials into the request context.
func New(_ any, log interfaces.Logger, adminGroupDN string, useUserBind bool) *Bundle {
	return &Bundle{
		log:          log,
		adminGroupDN: adminGroupDN,
		useUserBind:  useUserBind,
	}
}
