package server

import (
	"sync"
	"time"
)

type cacheItem struct {
	data      []byte
	expiresAt time.Time
}

var (
	apiCacheMu sync.RWMutex
	apiCache   = map[string]*cacheItem{}
)

func cacheGet(key string) []byte {
	apiCacheMu.RLock()
	item, ok := apiCache[key]
	apiCacheMu.RUnlock()
	if !ok {
		return nil
	}
	if time.Now().After(item.expiresAt) {
		apiCacheMu.Lock()
		delete(apiCache, key)
		apiCacheMu.Unlock()
		return nil
	}
	return item.data
}

func cacheSet(key string, data []byte, ttl time.Duration) {
	apiCacheMu.Lock()
	apiCache[key] = &cacheItem{data: data, expiresAt: time.Now().Add(ttl)}
	apiCacheMu.Unlock()
}

func cacheClear() {
	apiCacheMu.Lock()
	apiCache = map[string]*cacheItem{}
	apiCacheMu.Unlock()
}
