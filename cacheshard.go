package easycache

import (
	"container/list"
	"sync"
	"time"
)

const (
	defaultInternal = 1 * time.Hour // 这个定时器可以间隔长些
)

type cacheShard struct {
	lock sync.RWMutex

	// cache
	items       map[string]*list.Element // all  k/v
	expireItems map[string]*list.Element // expire k/v  optimize：reduce the number of expire keys scanned
	list        *list.List
	cap         uint32 // cache size

	// log
	logger    Logger
	isVerbose bool

	// timer
	cleanupTicker   *time.Ticker
	cleanupInterval time.Duration

	// add notify
	addChan chan string

	id       int
	onRemove OnRemoveCallback
	// close
	close chan struct{}
}

func newCacheShard(conf Config, id int, onRemove OnRemoveCallback, close chan struct{}) *cacheShard {

	shard := &cacheShard{
		items:           make(map[string]*list.Element),
		expireItems:     make(map[string]*list.Element),
		cap:             conf.Cap,
		list:            list.New(),
		logger:          newLogger(conf.Logger),
		cleanupInterval: defaultInternal,
		cleanupTicker:   time.NewTicker(defaultInternal),
		addChan:         make(chan string),
		isVerbose:       conf.Verbose,
		id:              id,
		onRemove:        onRemove,
		close:           close,
	}

	go shard.expireCleanup()
	return shard
}

func (cs *cacheShard) flush() {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	cs.cleanupTicker.Stop()
	cs.expireItems = nil
	cs.items = nil
	cs.list = nil
}
func (cs *cacheShard) expireCleanup() {

	for {
		select {
		case <-cs.cleanupTicker.C:
		case k := <-cs.addChan: // 立即触发
			if cs.isVerbose {
				cs.logger.Printf("[shard %d]: set expired key <%s>", cs.id, k)
			}

		case <-cs.close: // stop goroutine
			if cs.isVerbose {
				cs.logger.Printf("[shard %d] flush..", cs.id)
			}
			cs.flush() // free
			return
		}
		cs.cleanupTicker.Stop()

		smallestInternal := 0 * time.Second
		now := time.Now()
		cs.lock.Lock()

		for key, ele := range cs.expireItems { // 遍历

			item := ele.Value.(*cacheItem)
			if item.LifeSpan() == 0 { // 没有过期时间
				cs.logger.Printf("warning wrong data\n")
				continue
			}

			if now.Sub(item.CreatedOn()) >= item.LifeSpan() { // 过期
				// del
				delete(cs.items, key)
				delete(cs.expireItems, key)
				cs.list.Remove(ele)
				cs.onRemove(key, item.Value(), Expired)

				if cs.isVerbose {
					cs.logger.Printf("[shard %d]: expire del key <%s>  createdOn:%v,  lifeSpan:%d ms \n", cs.id, key, item.CreatedOn(), item.LifeSpan().Milliseconds())
				}
			} else {
				d := item.LifeSpan() - now.Sub(item.CreatedOn())
				if smallestInternal == 0 || d < smallestInternal {
					smallestInternal = d
				}
			}

		}

		if smallestInternal == 0 {
			smallestInternal = defaultInternal
		}
		cs.cleanupInterval = smallestInternal
		cs.cleanupTicker.Reset(cs.cleanupInterval)
		cs.lock.Unlock()
	}
}

func (cs *cacheShard) set(key string, value interface{}, lifeSpan time.Duration) error {

	cs.lock.Lock()
	defer cs.lock.Unlock()

	oldEle, ok := cs.items[key]
	if ok { // old item
		oldItem := oldEle.Value.(*cacheItem)
		oldLifeSpan := oldItem.LifeSpan()

		// modify
		oldEle.Value = newCacheItem(key, value, lifeSpan)
		cs.list.MoveToFront(oldEle)

		if oldLifeSpan > 0 && lifeSpan == 0 { // 原来的有过期时间，新的没有过期时间
			if cs.isVerbose {
				cs.logger.Printf("[shard %d] reset persist key <%s>\n", cs.id, key)
			}
			delete(cs.expireItems, key)
		}

		if oldLifeSpan == 0 && lifeSpan > 0 {
			cs.expireItems[key] = oldEle
			if lifeSpan < cs.cleanupInterval {
				go func() {
					cs.addChan <- key
				}()
			}
		}

	} else { // new item

		if len(cs.items) >= int(cs.cap) { // No space
			delVal := cs.list.Remove(cs.list.Back())
			item := delVal.(*cacheItem)
			delete(cs.items, item.Key())
			if item.LifeSpan() > 0 {
				delete(cs.expireItems, item.Key())
			}
			cs.onRemove(key, item.Value(), NoSpace)

			if cs.isVerbose {
				cs.logger.Printf("[shard %d] no space del key <%s>\n", cs.id, item.Key())
			}
		}
		// add
		ele := cs.list.PushFront(newCacheItem(key, value, lifeSpan))
		cs.items[key] = ele
		if lifeSpan > 0 {
			cs.expireItems[key] = ele
			if lifeSpan < cs.cleanupInterval {
				go func() {
					cs.addChan <- key
				}()
			}
		}
	}
	return nil
}

func (cs *cacheShard) get(key string) (interface{}, error) {
	cs.lock.RLock()
	defer cs.lock.RUnlock()
	ele, ok := cs.items[key]
	if ok {
		cs.list.MoveToFront(ele) // lru : move to front
		return ele.Value.(*cacheItem).Value(), nil
	}

	return nil, ErrKeyNotExist
}

func (cs *cacheShard) del(key string) error {

	cs.lock.Lock()
	defer cs.lock.Unlock()
	ele, ok := cs.items[key]
	if !ok {
		return ErrKeyNotExist
	}

	delete(cs.items, key)
	val := cs.list.Remove(ele)

	item := val.(*cacheItem)
	if item.LifeSpan() > 0 {
		delete(cs.expireItems, key)
	}
	cs.onRemove(key, item.Value(), Deleted)
	if cs.isVerbose {
		cs.logger.Printf("[shard %d] manual del key <%s>\n", cs.id, key)
	}
	return nil

}

func (cs *cacheShard) count() int {
	cs.lock.RLock()
	defer cs.lock.RUnlock()
	return len(cs.items)
}

func (cs *cacheShard) foreach(f func(key string, value interface{})) {

	cs.lock.RLock()
	defer cs.lock.RUnlock()

	for key, ele := range cs.items {
		f(key, ele.Value.(*cacheItem).Value())
	}
}

func (cs *cacheShard) exists(key string) bool {
	cs.lock.RLock()
	defer cs.lock.RUnlock()
	_, ok := cs.items[key]
	return ok
}

func (cs *cacheShard) check() bool {

	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return len(cs.items) == cs.list.Len() && len(cs.items) <= int(cs.cap)
}

func (cs *cacheShard) getorset(key string, value interface{}, lifeSpan time.Duration) (interface{}, error) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	oldEle, ok := cs.items[key]
	if ok {
		cs.list.MoveToFront(oldEle) // lru : move to front
		return oldEle.Value.(*cacheItem).Value(), nil
	}

	if len(cs.items) >= int(cs.cap) { // No space
		delVal := cs.list.Remove(cs.list.Back())
		item := delVal.(*cacheItem)
		delete(cs.items, item.Key())
		if item.LifeSpan() > 0 {
			delete(cs.expireItems, item.Key())
		}
		cs.onRemove(key, item.Value(), NoSpace)
	}
	// add
	ele := cs.list.PushFront(newCacheItem(key, value, lifeSpan))
	cs.items[key] = ele
	if lifeSpan > 0 {
		cs.expireItems[key] = ele
		if lifeSpan < cs.cleanupInterval {
			go func() {
				cs.addChan <- key
			}()
		}
	}
	return value, nil
}
