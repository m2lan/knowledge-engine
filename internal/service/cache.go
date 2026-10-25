package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/omnidev/knowledge-engine/types"
)

const searchVersionTTL = 24 * time.Hour

// kbSearchVersion returns the current cache generation for a knowledge base.
// Bumping the generation invalidates every cached search for that KB without
// scanning keys (works for both Redis and the in-memory cache).
func kbSearchVersion(ctx context.Context, cache types.Cache, kbID uuid.UUID) string {
	if cache == nil {
		return "0"
	}
	key := "ke:searchver:" + kbID.String()
	value, err := cache.Get(ctx, key)
	if err != nil || len(value) == 0 {
		return "0"
	}
	return string(value)
}

func invalidateSearchCache(ctx context.Context, cache types.Cache, kbID uuid.UUID) {
	if cache == nil {
		return
	}
	key := "ke:searchver:" + kbID.String()
	generation := fmt.Sprintf("%d", time.Now().UnixNano())
	_ = cache.Set(ctx, key, []byte(generation), searchVersionTTL)
}

func searchCacheKey(kbID uuid.UUID, generation, model, query string, topK int, minScore float64) string {
	payload := fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%f", model, query, generation, topK, minScore)
	sum := sha256.Sum256([]byte(payload))
	return "ke:search:" + kbID.String() + ":" + hex.EncodeToString(sum[:16])
}
