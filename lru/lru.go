package lru

import "container/list"

//这是一个LRU缓存，它不适合并发访问
type Cache struct {
	maxBytes int64
	nbytes   int64
	ll       *list.List
	cache    map[string]*list.Element
	// 可选，在条目被清除时执行
	onEvicted func(key string, value Value)
}

type Value interface {
	Len() int64
}

//双向链表节点的数据类型
type entry struct {
	key   string
	value Value
}

func New(maxBytes int64, onEvicted func(key string, value Value)) *Cache {
	return &Cache{
		maxBytes:  maxBytes,
		ll:        list.New(),
		cache:     make(map[string]*list.Element),
		onEvicted: onEvicted,
	}
}

//查找功能
func (c *Cache) Get(key string) (value Value, ok bool) {
	// 如果key存在，返回true
	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		return kv.value, true
	}
	// 如果key不存在，返回false
	return
}

//缓存淘汰功能
// RemoveOldest 会移除最旧的项目
func (c *Cache) RemoveOldest() {
	ele := c.ll.Back()
	if ele != nil {
		c.ll.Remove(ele)
		kv := ele.Value.(*entry)
		delete(c.cache, kv.key)
		c.nbytes -= int64(len(kv.key)) + int64(kv.value.Len())
		if c.onEvicted != nil {
			c.onEvicted(kv.key, kv.value)
		}
	}
}

//更新/添加 功能
func (c *Cache) Add(key string, value Value) {
	// 如果key存在，更新value
	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		c.nbytes += value.Len() - kv.value.Len()
		kv.value = value
		return
	} else {
		// 如果key不存在，添加到缓存
		ele := c.ll.PushFront(&entry{key, value})
		c.cache[key] = ele
		c.nbytes += int64(len(key)) + value.Len()
	}

	for c.maxBytes != 0 && c.nbytes > c.maxBytes {
		c.RemoveOldest()
	}
}

func (c *Cache) Len() int {
	return c.ll.Len()
}
