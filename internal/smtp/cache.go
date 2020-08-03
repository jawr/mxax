package smtp

import (
	"fmt"
	"time"
)

const DefaultCacheCost int64 = 1
const DefaultCacheTTL time.Duration = time.Minute

func (s *Server) cacheGet(namespace, key string) (interface{}, bool) {
	v, ok := s.cache.Get(fmt.Sprintf("%s:%s", namespace, key))
	return v, ok
}

func (s *Server) cacheSet(namespace, key string, v interface{}) {
	s.cache.SetWithTTL(fmt.Sprintf("%s:%s", namespace, key), v, DefaultCacheCost, DefaultCacheTTL)
}
