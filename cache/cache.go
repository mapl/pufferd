package cache

import (
	"github.com/pufferpanel/apufferi/cache"
	"github.com/pufferpanel/apufferi/config"
)

func CreateCache() *cache.MemoryCache {
	capacity := config.GetIntOrDefault("console-buffer", 50)
	return &cache.MemoryCache{
		Buffer:   make([]cache.Message, 0),
		Capacity: capacity,
	}
}
