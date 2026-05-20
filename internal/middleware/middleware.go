package middleware

import "ad-sync-manager/internal/domain/interfaces"

// Bundle groups all middleware constructors so handlers.NewRouter receives
// a single typed dependency rather than many loose functions.
//
// Stage 2 note: the sessionSvc field has been removed. JWT validation is now
// handled entirely by the auth package (auth.ValidateJWT). The Bundle retains
// the Logger and adminGroupDN fields used by RequireAdmin and RequestLogger.
type Bundle struct {
	log          interfaces.Logger
	adminGroupDN string
}

// New constructs a middleware Bundle.
// sessionSvc is no longer needed in Stage 2 and is accepted as nil for
// backwards-compatibility with callers that pass it; it is ignored.
func New(_ any, log interfaces.Logger, adminGroupDN string) *Bundle {
	return &Bundle{
		log:          log,
		adminGroupDN: adminGroupDN,
	}
}
