package auth

import (
	"sync"
	"time"
)

// credentialCache is an in-memory store mapping a raw JWT token string to the
// user's LDAP bind credentials for the duration of their session.
//
// Security note: passwords are held as plaintext in process memory only for
// the JWT lifetime. They are never written to disk, logged, or transmitted.
// Used exclusively when AD_USE_USER_BIND=true.
type credentialCache struct {
	mu    sync.RWMutex
	items map[string]cachedCred
}

type cachedCred struct {
	DN        string
	Password  string
	ExpiresAt time.Time
}

var globalCredCache = &credentialCache{
	items: make(map[string]cachedCred),
}

// StoreCredential saves the user's LDAP bind credentials keyed by their raw
// JWT token. Should be called immediately after a successful login when
// AD_USE_USER_BIND=true.
func StoreCredential(rawToken, dn, password string, exp time.Time) {
	globalCredCache.mu.Lock()
	globalCredCache.items[rawToken] = cachedCred{DN: dn, Password: password, ExpiresAt: exp}
	globalCredCache.mu.Unlock()
}

// LookupCredential retrieves stored credentials for the given token.
// Returns false if the token is unknown or has expired.
func LookupCredential(rawToken string) (LDAPCred, bool) {
	globalCredCache.mu.RLock()
	item, ok := globalCredCache.items[rawToken]
	globalCredCache.mu.RUnlock()

	if !ok {
		return LDAPCred{}, false
	}
	if time.Now().After(item.ExpiresAt) {
		globalCredCache.mu.Lock()
		delete(globalCredCache.items, rawToken)
		globalCredCache.mu.Unlock()
		return LDAPCred{}, false
	}
	return LDAPCred{DN: item.DN, Password: item.Password}, true
}

// DeleteCredential removes a token's credentials from the cache (called on logout).
func DeleteCredential(rawToken string) {
	globalCredCache.mu.Lock()
	delete(globalCredCache.items, rawToken)
	globalCredCache.mu.Unlock()
}
