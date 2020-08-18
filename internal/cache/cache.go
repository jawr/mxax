package cache

import (
	"fmt"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/pkg/errors"
)

const DefaultCacheCost int64 = 1
const DefaultCacheTTL time.Duration = time.Minute

type Cache struct {
	c *ristretto.Cache
}

func NewCache() (*Cache, error) {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	c := &Cache{
		c: cache,
	}

	return c, nil
}

func (c *Cache) Get(namespace, key string) (interface{}, bool) {
	v, ok := c.c.Get(fmt.Sprintf("%s:%s", namespace, key))
	return v, ok
}

func (c *Cache) Set(namespace, key string, v interface{}) {
	c.c.SetWithTTL(fmt.Sprintf("%s:%s", namespace, key), v, DefaultCacheCost, DefaultCacheTTL)
}
