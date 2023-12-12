package easycache

import (
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/gofish2020/easycache/utils"
)

func TestSetAndGet(t *testing.T) {
	t.Parallel()

	// given
	cache, _ := New(DefaultConfig())
	value := "value"

	// when
	cache.Set("key", value, 0*time.Second)
	cachedValue, err := cache.Get("key")

	// then
	noError(t, err)
	assertEqual(t, value, cachedValue)
}

func TestSetAndDelayGet(t *testing.T) {
	t.Parallel()

	cache, _ := New(TestConfig())

	key := "easy_key"
	value := utils.RandomValue(10)
	err := cache.Set(key, value, 3*time.Second)
	noError(t, err)

	cacheValue, _ := cache.Get(key)
	assertEqual(t, value, cacheValue)

	time.Sleep(4 * time.Second)

	_, err = cache.Get(key)
	assertEqual(t, ErrKeyNotExist, err)
}

func TestSetOverCap(t *testing.T) {
	t.Parallel()

	cache, _ := New(TestConfig())
	for i := 0; i < 4; i++ { // lru 【2 0】 【3 1】
		key := strconv.Itoa(i)
		cache.Set(key, i, 0*time.Second)
	}

	cache.Get("0") // lru 【0 2】 【3 1】
	cache.Get("1") // lru 【0 2】 【1 3】

	cache.Set("4", 4, 0*time.Second) // del 2  lru [4 0] [1 3]
	cache.Set("5", 4, 0*time.Second) // del 3  lru [4 0] [5 1]

	_, err := cache.Get("2")
	assertEqual(t, ErrKeyNotExist, err)

	_, err = cache.Get("3")
	assertEqual(t, ErrKeyNotExist, err)
}

func TestSetSameKeyDiffExpireTime(t *testing.T) {

	t.Parallel()

	cache, _ := New(TestConfig())

	rand.Seed(time.Now().UnixNano())

	key := string(utils.RandomValue(20))

	// same key
	cache.Set(key, 0, 5*time.Second) // expire 5s
	cache.Set(key, 0, 0*time.Second) // expire 0s

	// same key1
	key1 := string(utils.RandomValue(20))
	cache.Set(key1, 1, 0*time.Second) // expire 0s
	cache.Set(key1, 1, 5*time.Second) // expire 5s

	time.Sleep(6 * time.Second)

	// key
	cacheVal, _ := cache.Get(key)
	assertEqual(t, 0, cacheVal)

	// key1
	_, err := cache.Get(key1)
	assertEqual(t, ErrKeyNotExist, err)
}

func TestGetOrSet(t *testing.T) {

	t.Parallel()

	cache, _ := New(TestConfig())

	key := string(utils.RandomValue(20))
	cache.GetOrSet(key, 0, 5*time.Second) // expire 5s
	time.Sleep(6 * time.Second)
	cache.GetOrSet(key, 1, 0*time.Second) // persist
	time.Sleep(6 * time.Second)

	cacheVal, _ := cache.Get(key)

	assertEqual(t, 1, cacheVal)

}

func TestDelete(t *testing.T) {

	t.Parallel()
	cache, _ := New(TestConfig())

	key := string(utils.RandomValue(20))
	cache.Set(key, 0, 0*time.Second)
	err := cache.Delete(key) // del persist key
	assertEqual(t, nil, err)

	key1 := string(utils.RandomValue(20))
	cache.Set(key1, 0, 2*time.Second)
	time.Sleep(3 * time.Second)
	err = cache.Delete(key1) // del expire key
	assertEqual(t, ErrKeyNotExist, err)
}

func TestRemoveCallback(t *testing.T) {

	t.Parallel()

	conf := TestConfig()
	conf.OnRemoveWithReason = func(key string, value interface{}, reason RemoveReason) {
		t.Logf("key <%s> del reason is <%d>\n", key, reason)
	}
	cache, _ := New(conf)
	key := string(utils.RandomValue(20))
	cache.Set(key, 0, 2*time.Second)
	key1 := string(utils.RandomValue(20))
	cache.Set(key1, 0, 0*time.Second)
	cache.Delete(key1)

	time.Sleep(3 * time.Second)

}
