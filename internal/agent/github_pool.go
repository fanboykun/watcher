package agent

import (
	"crypto/sha256"
	"sync"
)

// GitHubPool shares rate-limit state per token, because GitHub enforces quota per
// token (or per IP when anonymous), not per watcher or per poll cycle.
type GitHubPool struct {
	mu       sync.Mutex
	limiters map[[sha256.Size]byte]*rateLimiter
}

// defaultGitHubPool backs NewGitHubClient so every caller in the process —
// watchers, API handlers, catalog and self-update — shares one view of each quota.
var defaultGitHubPool = NewGitHubPool()

// NewGitHubPool creates an empty pool; production code uses the default pool.
func NewGitHubPool() *GitHubPool {
	return &GitHubPool{limiters: make(map[[sha256.Size]byte]*rateLimiter)}
}

// Client returns a client bound to the pooled state for token. Clients are cheap;
// callers keep their own logger. Tokens are keyed by hash and never stored raw.
func (p *GitHubPool) Client(token string, log *Logger) *GitHubClient {
	return newGitHubClient(token, log, p.limiter(token))
}

func (p *GitHubPool) limiter(token string) *rateLimiter {
	key := sha256.Sum256([]byte(token))
	p.mu.Lock()
	defer p.mu.Unlock()
	limiter, ok := p.limiters[key]
	if !ok {
		limiter = newRateLimiter()
		p.limiters[key] = limiter
	}
	return limiter
}
