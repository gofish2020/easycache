package easycache

import (
	"fmt"
)

type RemoveReason uint32

const (
	// Expired means the key is past its LifeWindow.
	Expired = RemoveReason(1)
	// NoSpace means the key is the oldest and the cache size was at its maximum when Set was called, or the
	// entry exceeded the maximum shard size.
	NoSpace = RemoveReason(2)
	// Deleted means Delete was called and this key was removed as a result.
	Deleted = RemoveReason(3)
)

type OnRemoveCallback func(key string, value interface{}, reason RemoveReason)

type Config struct {
	// Number of cache shards, value must be a power of two
	Shards int
	// Number of key in signal cache shards
	Cap uint32
	// Hasher used to map between string keys and unsigned 64bit integers, by default fnv64 hashing is used.
	Hasher Hasher

	Logger  Logger
	Verbose bool

	OnRemoveWithReason OnRemoveCallback
}

func DefaultConfig() Config {
	return Config{
		Shards:  1024,
		Cap:     32,
		Hasher:  newDefaultHasher(),
		Logger:  DefaultLogger(),
		Verbose: false,
	}
}

func TestConfig() Config {
	return Config{
		Shards:  2,
		Cap:     2,
		Hasher:  newDefaultHasher(),
		Logger:  DefaultLogger(),
		Verbose: true,
		OnRemoveWithReason: func(key string, value interface{}, reason RemoveReason) {
			fmt.Printf("execute callback  key:<%s> value:<%+v> reason:<%d>\n", key, value, reason)
		},
	}
}
