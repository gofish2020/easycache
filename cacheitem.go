package easycache

import "time"

type cacheItem struct {
	key       string
	value     interface{}
	lifeSpan  time.Duration // 存储时长
	createdOn time.Time
}

func newCacheItem(key string, value interface{}, duration time.Duration) *cacheItem {

	item := &cacheItem{
		key:       key,
		value:     value,
		lifeSpan:  duration,
		createdOn: time.Now(),
	}

	return item
}

func (i cacheItem) LifeSpan() time.Duration {
	return i.lifeSpan
}

func (i cacheItem) CreatedOn() time.Time {
	return i.createdOn
}

func (i cacheItem) Key() string {
	return i.key
}

func (i cacheItem) Value() interface{} {
	return i.value
}
