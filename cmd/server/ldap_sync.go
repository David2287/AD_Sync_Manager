package main

import (
	"ad-sync-manager/internal/config"
	adrepo "ad-sync-manager/internal/repositories/ad"
	"ad-sync-manager/internal/domain/interfaces"
)

// newLDAPSyncClient opens the service-account LDAP connection used exclusively
// for employee sync (SyncEmployees, GetEmployee). Authentication is handled by
// the auth package independently, which opens its own connections per request.
func newLDAPSyncClient(cfg *config.Config, log interfaces.Logger) (interfaces.ADClient, error) {
	client, err := adrepo.NewLDAPClient(cfg.AD)
	if err != nil {
		log.Error("LDAP sync client init failed", "error", err, "url", cfg.AD.URL)
		return nil, err
	}
	log.Info("LDAP sync client connected", "url", cfg.AD.URL)
	return client, nil
}
