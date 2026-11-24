package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// DomainCacheEntry represents a 30-day domain verification cache entry.
type DomainCacheEntry struct {
	Domain         string    `json:"domain"`
	TrustScore     int       `json:"trust_score"`
	TrustReasons   []string  `json:"trust_reasons"`
	CompanySummary string    `json:"company_summary"`
	RedFlags       []string  `json:"red_flags"`
	HasMX          bool      `json:"has_mx"`
	IsPersonal     bool      `json:"is_personal"`
	VerifiedAt     time.Time `json:"verified_at"`
}

// DomainCache manages domain verification caching for 30 days.
type DomainCache struct {
	mu     sync.RWMutex
	memory map[string]DomainCacheEntry
	store  SettingsGetterSaver
}

// SettingsGetterSaver defines persistence methods for domain cache entries.
type SettingsGetterSaver interface {
	GetSettingRaw(ctx context.Context, key string) ([]byte, error)
	SaveSettingRaw(ctx context.Context, key string, value []byte) error
}

// NewDomainCache creates a DomainCache.
func NewDomainCache(store SettingsGetterSaver) *DomainCache {
	return &DomainCache{
		memory: make(map[string]DomainCacheEntry),
		store:  store,
	}
}

// Get checks if a valid cached verification result exists within 30 days.
func (c *DomainCache) Get(ctx context.Context, domain string) (*DomainCacheEntry, bool) {
	c.mu.RLock()
	entry, ok := c.memory[domain]
	c.mu.RUnlock()

	thirtyDaysAgo := time.Now().UTC().Add(-30 * 24 * time.Hour)

	if ok && entry.VerifiedAt.After(thirtyDaysAgo) {
		return &entry, true
	}

	// Try loading from persistent store if available
	if c.store != nil {
		key := fmt.Sprintf("domain_cache:%s", domain)
		raw, err := c.store.GetSettingRaw(ctx, key)
		if err == nil && len(raw) > 0 {
			var persisted DomainCacheEntry
			if err := json.Unmarshal(raw, &persisted); err == nil {
				if persisted.VerifiedAt.After(thirtyDaysAgo) {
					c.mu.Lock()
					c.memory[domain] = persisted
					c.mu.Unlock()
					return &persisted, true
				}
			}
		}
	}

	return nil, false
}

// Put stores a domain verification result in cache for 30 days.
func (c *DomainCache) Put(ctx context.Context, entry DomainCacheEntry) {
	if entry.VerifiedAt.IsZero() {
		entry.VerifiedAt = time.Now().UTC()
	}

	c.mu.Lock()
	c.memory[entry.Domain] = entry
	c.mu.Unlock()

	if c.store != nil {
		key := fmt.Sprintf("domain_cache:%s", entry.Domain)
		if raw, err := json.Marshal(entry); err == nil {
			_ = c.store.SaveSettingRaw(ctx, key, raw)
		}
	}
}
