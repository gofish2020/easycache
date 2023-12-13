package easycache

import (
	"errors"
	"time"

	"github.com/gofish2020/easycache/utils"
)

const (
	defaultCap = 32
)

type Getter interface {
	Get(string) (interface{}, error)
}

type GetterFunc func(string) (interface{}, error)

func (g GetterFunc) Get(key string) (interface{}, error) {
	return g(key)
}

type EasyCache struct {
	shards    []*cacheShard
	hash      Hasher
	conf      Config
	shardMask uint64 // mask

	close chan struct{}
}

// New initialize new instance of EasyCache
func New(conf Config) (*EasyCache, error) {

	if !utils.IsPowerOfTwo(conf.Shards) {
		return nil, errors.New("shards number must be power of two")
	}

	if conf.Cap <= 0 {
		conf.Cap = defaultCap
	}
	// init cache object
	cache := &EasyCache{
		shards:    make([]*cacheShard, conf.Shards),
		conf:      conf,
		hash:      conf.Hasher,
		shardMask: uint64(conf.Shards - 1), // mask
		close:     make(chan struct{}),
	}

	var onRemove OnRemoveCallback
	if conf.OnRemoveWithReason != nil {
		onRemove = conf.OnRemoveWithReason
	} else {
		onRemove = cache.notProvidedOnRemove
	}

	// init shard
	for i := 0; i < conf.Shards; i++ {
		cache.shards[i] = newCacheShard(conf, i, onRemove, cache.close)
	}
	return cache, nil
}

// Set add k/v or modify existing k/v
func (e *EasyCache) Set(key string, value interface{}, duration time.Duration) error {

	hashedKey := e.hash.Sum64(key)
	shard := e.getShard(hashedKey)
	return shard.set(key, value, duration)

}

// Get get k/v if exist,otherwise get an error
func (e *EasyCache) Get(key string) (interface{}, error) {
	hashedKey := e.hash.Sum64(key)
	shard := e.getShard(hashedKey)
	return shard.get(key)
}

// GetIfNotExist get an existing k/v  or  create a new k/v though by Getter
func (e *EasyCache) GetIfNotExist(key string, g Getter, duration time.Duration) (interface{}, error) {
	hashedKey := e.hash.Sum64(key)
	shard := e.getShard(hashedKey)
	return shard.getIfNotExist(key, g, duration)
}

func (e *EasyCache) GetOrSet(key string, value interface{}, duration time.Duration) (interface{}, error) {
	hashedKey := e.hash.Sum64(key)
	shard := e.getShard(hashedKey)
	return shard.getorset(key, value, duration)
}

func (e *EasyCache) Delete(key string) error {
	hashedKey := e.hash.Sum64(key)
	shard := e.getShard(hashedKey)
	return shard.del(key)
}

func (e *EasyCache) Count() int {
	count := 0
	for _, shard := range e.shards {
		count += shard.count()
	}
	return count
}

func (e *EasyCache) Foreach(f func(key string, value interface{})) {
	for _, shard := range e.shards {
		shard.foreach(f)
	}
}

func (e *EasyCache) Exists(key string) bool {
	hashedKey := e.hash.Sum64(key)
	shard := e.getShard(hashedKey)
	return shard.exists(key)
}

func (e *EasyCache) Close() error {
	close(e.close)
	return nil
}
func (e *EasyCache) getShard(hashedKey uint64) (shard *cacheShard) {
	return e.shards[hashedKey&e.shardMask]
}

func (e *EasyCache) notProvidedOnRemove(key string, value interface{}, reason RemoveReason) {
}
