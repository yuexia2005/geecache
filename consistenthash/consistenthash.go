package consistenthash

import (
	"hash/crc32"
	"sort"
	"strconv"
)

// 将任意长度的字节序列映射为一个 uint32 整数。
type Hash func(data []byte) uint32

// 这个 Map 结构体存储了一致性哈希环中所有节点的哈希值
type Map struct {
	hash Hash
	//虚拟节点数
	replicas int
	keys     []int
	//虚拟节点与真实节点的映射表
	hashMap map[int]string
}

// 构造函数 New() 新建一个 Map 实例
func New(replicas int, fn Hash) *Map {
	m := &Map{
		replicas: replicas,
		hash:     fn,
		hashMap:  make(map[int]string),
	}
	if m.hash == nil {
		m.hash = crc32.ChecksumIEEE
	}
	return m
}

// 添加一些节点到哈希表中
func (m *Map) Add(keys ...string) {
	for _, key := range keys {
		for i := 0; i < m.replicas; i++ {
			hash := int(m.hash([]byte(strconv.Itoa(i) + key)))
			m.keys = append(m.keys, hash)
			m.hashMap[hash] = key
		}
	}
	sort.Ints(m.keys)

}

// Get 函数获取哈希表中与提供的键最接近的项。
func (m *Map) Get(key string) string {
	if len(m.keys) == 0 {
		return ""
	}

	hash := int(m.hash([]byte(key)))
	// 使用二分查找法查找合适的节点
	idx := sort.Search(len(m.keys), func(i int) bool {
		return m.keys[i] >= hash
	})
	//处理若hash最大情况，返回物理节点名字
	return m.hashMap[m.keys[idx%len(m.keys)]]
}
