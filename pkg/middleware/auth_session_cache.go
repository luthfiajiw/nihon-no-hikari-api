package middleware

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

const defaultSessionCacheMax = 10_000

type sessionEntry struct {
	userID    uuid.UUID
	role      string
	expiresAt time.Time
}

// sessionCache caches positive session validations in memory for a short TTL.
// Negative lookups and capacity overflows fall through to the database.
type sessionCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	max     int
	now     func() time.Time
	entries map[uuid.UUID]sessionEntry
}

func newSessionCache(ttl time.Duration, maxEntries int) *sessionCache {
	if maxEntries <= 0 {
		maxEntries = defaultSessionCacheMax
	}
	return &sessionCache{
		ttl:     ttl,
		max:     maxEntries,
		now:     time.Now,
		entries: make(map[uuid.UUID]sessionEntry),
	}
}

func (c *sessionCache) get(sessionID, userID uuid.UUID) (string, bool) {
	if c == nil || c.ttl <= 0 {
		return "", false
	}
	now := c.now()
	c.mu.RLock()
	entry, ok := c.entries[sessionID]
	c.mu.RUnlock()
	if !ok || entry.userID != userID || !now.Before(entry.expiresAt) {
		return "", false
	}
	return entry.role, true
}

func (c *sessionCache) put(sessionID, userID uuid.UUID, role string, sessionExpiry time.Time) {
	if c == nil || c.ttl <= 0 {
		return
	}
	now := c.now()
	expiresAt := now.Add(c.ttl)
	if !sessionExpiry.IsZero() && sessionExpiry.Before(expiresAt) {
		expiresAt = sessionExpiry
	}
	if !now.Before(expiresAt) {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.max {
		if _, exists := c.entries[sessionID]; !exists {
			for k, v := range c.entries {
				if !now.Before(v.expiresAt) {
					delete(c.entries, k)
				}
			}
			if len(c.entries) >= c.max {
				return
			}
		}
	}
	c.entries[sessionID] = sessionEntry{
		userID:    userID,
		role:      role,
		expiresAt: expiresAt,
	}
}

func (c *sessionCache) deleteSession(sessionID uuid.UUID) {
	if c == nil || sessionID == uuid.Nil {
		return
	}
	c.mu.Lock()
	delete(c.entries, sessionID)
	c.mu.Unlock()
}

func (c *sessionCache) deleteUser(userID uuid.UUID) {
	if c == nil || userID == uuid.Nil {
		return
	}
	c.mu.Lock()
	for k, v := range c.entries {
		if v.userID == userID {
			delete(c.entries, k)
		}
	}
	c.mu.Unlock()
}
