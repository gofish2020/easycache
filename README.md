# Golang实现缓存库 EasyCache


学了不吃亏，学了不上当,进厂打钉必备基本功，看完绝对有很爽的感觉。核心代码也就**300**多行，代码虽少但是功能一点不打折

托管到Github: https://github.com/gofish2020/easycache 欢迎 Fork & Star

## 通过本项目学到什么？

- Golang基础语法
- 缓存数据结构
- 锁的使用(并发安全 & 分片减小锁粒度)
- LRU(缓存淘汰算法)
- key过期删除策略（定时删除）
- 测试用例的编写

## 代码原理

`New`函数负责创建 `*EasyCache`对象，对象的底层包含 `conf.Shards`个分片，目的在于减少锁冲突
```go
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

```

`newCacheShard`函数，用来初始化实际存放 `k/v`的数据结构`*cacheShard`(也就是单个分片)。
分片底层的存储采用两个map和一个list:
-  `items`负责保存所有的`k/v`(过期or不过期都有存)
-  `expireItems`负责保存有过期时间的`k/v`，目的在于减少扫描key`的数据量
-  `list`用作`LRU`记录最近最少使用`key`的顺序。LRU代码实现看这篇文章 [Leetcode LRU题解](https://zhuanlan.zhihu.com/p/667020012)，有助于理解本项目中的LRU的细节。
```go
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

```

`expireCleanup`负责对本分片中过期的key进行定期删除：
代码理解的关键在于不同的`key`会有不同的过期时间，例如key=a 过期时间3s，key=b 过期时间5s。
- 定时器定时执行间隔不能太长，例如10s，a/b都已经过期了还不清理，太不及时
- 定时器定时执行间隔不能太短，例如1s，执行频率又太高了，a/b都未过期，空转
- 过期间隔肯定是动态变化的，一开始为3s间隔，执行后清理掉a，此时b还剩（5-3）=2s的存活时间，所以间隔再设定为2s。再执行完以后，没有数据了，那间隔就在设定一个大值`smallestInternal = defaultInternal`处于休眠状态

这里再思考一种情况，按照上述解释一开始间隔设定3s，等到过期了就可以将a清理掉。那如果用户这时又设定了key=c 过期时间1s，那如果定时器按照3s执行又变成了间隔太长了。所以我们需要发送信号`cs.addChan:`，重新设定过期间隔

```go

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
```

`set` 函数理解关键在于，用户可以对同一个key重复设定：

```go
cache.Set(key, 0, 5*time.Second) // expire 5s
cache.Set(key, 0, 0*time.Second) // expire 0s
```
第一次设定为5s过期，立刻又修改为0s不过期，所以在代码中需要判断key是否之前已经存在，
- 如果存在重复&有过期时间，需要从过期`expireItems`中剔除
- 如果不存在直接新增即可（前提：容量还有剩余）


LRU的基本规则：
- 最新数据放到list的Front
- 如果超过最大容量，从list的Back删除元素


```go

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

```