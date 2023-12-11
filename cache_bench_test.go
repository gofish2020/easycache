package easycache

import (
	"bytes"
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"
)

var message = blob('a', 256)

func blob(char byte, len int) []byte {
	return bytes.Repeat([]byte{char}, len)
}

func writeToCache(b *testing.B, shard int, cap uint32, repeat bool, noExpire bool) {

	cache, _ := New(Config{
		Shards:  shard,
		Cap:     cap,
		Hasher:  newDefaultHasher(),
		Logger:  DefaultLogger(),
		Verbose: false,
	})

	rand.Seed(time.Now().Unix())

	b.RunParallel(func(p *testing.PB) {
		id := rand.Int()
		counter := 0

		b.ReportAllocs()
		for p.Next() {

			if noExpire {
				cache.Set(fmt.Sprintf("key-%d-%d", id, counter), message, 0*time.Second)
			} else {
				cache.Set(fmt.Sprintf("key-%d-%d", id, counter), message, time.Duration(rand.Intn(b.N))*time.Second)
			}

			if repeat {
				counter = counter + 1
			}

		}
	})
}
func BenchmarkWriteToCache(b *testing.B) {
	for _, shards := range []int{1, 512, 1024, 2048} {
		for _, cap := range []uint32{32, 128, 512, 1024} {
			b.Run(fmt.Sprintf("%d-shards-%d-cap", shards, cap), func(b *testing.B) {
				writeToCache(b, shards, cap, false, false)
			})
		}
	}
}

func BenchmarkWriteToCacheNoExpire(b *testing.B) {
	for _, shards := range []int{1, 512, 1024, 2048} {
		for _, cap := range []uint32{32, 128, 512, 1024} {
			b.Run(fmt.Sprintf("%d-shards-%d-cap", shards, cap), func(b *testing.B) {
				writeToCache(b, shards, cap, false, true)
			})
		}
	}
}

func BenchmarkWriteToCacheRepeatKey(b *testing.B) {
	for _, shards := range []int{1, 512, 1024, 2048} {
		for _, cap := range []uint32{32, 128, 512, 1024} {
			b.Run(fmt.Sprintf("%d-shards-%d-cap", shards, cap), func(b *testing.B) {
				writeToCache(b, shards, cap, true, false)
			})
		}
	}
}

func BenchmarkWriteToCacheRepeatKeyNoExpire(b *testing.B) {
	for _, shards := range []int{1, 512, 1024, 2048} {
		for _, cap := range []uint32{32, 128, 512, 1024} {
			b.Run(fmt.Sprintf("%d-shards-%d-cap", shards, cap), func(b *testing.B) {
				writeToCache(b, shards, cap, true, true)
			})
		}
	}
}

func BenchmarkGetFromCache(b *testing.B) {
	for _, shards := range []int{1, 512, 1024, 2048} {
		for _, cap := range []uint32{32, 128, 512, 1024} {
			b.Run(fmt.Sprintf("%d-shards-%d-cap", shards, cap), func(b *testing.B) {
				readFromCache(b, shards, cap)
			})
		}
	}
}

func readFromCache(b *testing.B, shards int, cap uint32) {
	cache, _ := New(Config{
		Shards:  shards,
		Cap:     cap,
		Hasher:  newDefaultHasher(),
		Logger:  DefaultLogger(),
		Verbose: false,
	})
	for i := 0; i < b.N; i++ {
		cache.Set(strconv.Itoa(i), message, 0*time.Second)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		b.ReportAllocs()

		for pb.Next() {
			cache.Get(strconv.Itoa(rand.Intn(b.N)))
		}
	})
}

func BenchmarkReadFromCacheNonExistentKeys(b *testing.B) {
	for _, shards := range []int{1, 512, 1024, 8192} {

		for _, cap := range []uint32{32, 128, 512, 1024} {
			b.Run(fmt.Sprintf("%d-shards-%d-cap", shards, cap), func(b *testing.B) {
				readFromCacheNonExistentKeys(b, shards, cap)
			})
		}

	}
}

func readFromCacheNonExistentKeys(b *testing.B, shards int, cap uint32) {
	cache, _ := New(Config{
		Shards:  shards,
		Cap:     cap,
		Hasher:  newDefaultHasher(),
		Logger:  DefaultLogger(),
		Verbose: false,
	})
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		b.ReportAllocs()

		for pb.Next() {
			cache.Get(strconv.Itoa(rand.Intn(b.N)))
		}
	})
}

func BenchmarkGetOrSetCache(b *testing.B) {
	for _, shards := range []int{1, 512, 1024, 8192} {

		for _, cap := range []uint32{32, 128, 512, 1024} {
			b.Run(fmt.Sprintf("%d-shards-%d-cap", shards, cap), func(b *testing.B) {
				getOrSetCache(b, shards, cap)
			})
		}

	}
}

func getOrSetCache(b *testing.B, shards int, cap uint32) {
	cache, _ := New(Config{
		Shards:  shards,
		Cap:     cap,
		Hasher:  newDefaultHasher(),
		Logger:  DefaultLogger(),
		Verbose: false,
	})
	for i := 0; i < b.N/2; i++ {
		cache.Set(strconv.Itoa(i), message, 0*time.Second)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		b.ReportAllocs()

		for pb.Next() {

			randI := rand.Intn(b.N)
			cache.GetOrSet(strconv.Itoa(randI), message, time.Duration(randI)*time.Second)
		}
	})
}
