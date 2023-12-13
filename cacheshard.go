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

// shard
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
	// goroutine clean expired key
	go shard.expireCleanup()
	return shard
}

// clean cache
func (cs *cacheShard) flush() {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	cs.cleanupTicker.Stop()
	cs.expireItems = nil
	cs.items = nil
	cs.list = nil
}

/*
1.当定时器到期，执行过期清理
2.当新增的key有过期时间，通过addChan触发执行
*/
func (cs *cacheShard) expireCleanup() {

	for {
		select {
		case <-cs.cleanupTicker.C:
		case <-cs.addChan: // 立即触发

		case <-cs.close: // stop goroutine
			if cs.isVerbose {
				cs.logger.Printf("[shard %d] flush..", cs.id)
			}
			cs.flush() // free
			return
		}
		cs.cleanupTicker.Stop()

		// 记录下一次定时器的最小间隔（目的：key过期了，尽快删除）
		smallestInternal := 0 * time.Second
		now := time.Now()
		cs.lock.Lock()

		for key, ele := range cs.expireItems { // 遍历过期key

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
			delete(cs.expireItems, key)
		}

		if oldLifeSpan == 0 && lifeSpan > 0 { // 原有的无过期时间，当前有过期时间
			cs.expireItems[key] = oldEle
			if lifeSpan < cs.cleanupInterval {
				go func() {
					cs.addChan <- key
				}()
			}
		}

	} else { // new item

		if len(cs.items) >= int(cs.cap) { // lru: No space
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

	if cs.isVerbose {
		if lifeSpan == 0 {
			cs.logger.Printf("[shard %d]: set persist key <%s>\n", cs.id, key)
		} else {
			cs.logger.Printf("[shard %d]: set expired key <%s>", cs.id, key)
		}
	}
	return nil
}

func (cs *cacheShard) getIfNotExist(key string, g Getter, lifeSpan time.Duration) (interface{}, error) {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	oldEle, ok := cs.items[key]
	if ok {
		cs.list.MoveToFront(oldEle) // lru : move to front
		return oldEle.Value.(*cacheItem).Value(), nil
	}

	value, err := g.Get(key)
	if err != nil {
		return nil, err
	}

	// set
	if len(cs.items) >= int(cs.cap) { //lru: No space
		delVal := cs.list.Remove(cs.list.Back())
		item := delVal.(*cacheItem)
		delete(cs.items, item.Key())
		if item.LifeSpan() > 0 {
			delete(cs.expireItems, item.Key())
		}
		if cs.isVerbose {
			cs.logger.Printf("[shard %d] no space del key <%s>\n", cs.id, item.Key())
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
	// log
	if cs.isVerbose {
		if lifeSpan == 0 {
			cs.logger.Printf("[shard %d]: set persist key <%s>\n", cs.id, key)
		} else {
			cs.logger.Printf("[shard %d]: set expired key <%s>", cs.id, key)
		}
	}
	return value, nil

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

	// del items
	delete(cs.items, key)
	// del list
	val := cs.list.Remove(ele)
	item := val.(*cacheItem)
	// del expireItems
	if item.LifeSpan() > 0 {
		delete(cs.expireItems, key)
	}
	// remove callback
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

	// range all item
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

func (cs *cacheShard) getorset(key string, value interface{}, lifeSpan time.Duration) (interface{}, error) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	// get
	oldEle, ok := cs.items[key]
	if ok {
		cs.list.MoveToFront(oldEle) // lru : move to front
		return oldEle.Value.(*cacheItem).Value(), nil
	}

	// set
	if len(cs.items) >= int(cs.cap) { //lru: No space
		delVal := cs.list.Remove(cs.list.Back())
		item := delVal.(*cacheItem)
		delete(cs.items, item.Key())
		if item.LifeSpan() > 0 {
			delete(cs.expireItems, item.Key())
		}
		if cs.isVerbose {
			cs.logger.Printf("[shard %d] no space del key <%s>\n", cs.id, item.Key())
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
	// log
	if cs.isVerbose {
		if lifeSpan == 0 {
			cs.logger.Printf("[shard %d]: set persist key <%s>\n", cs.id, key)
		} else {
			cs.logger.Printf("[shard %d]: set expired key <%s>", cs.id, key)
		}
	}
	return value, nil
}
