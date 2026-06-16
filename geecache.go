package geecache

import (
	"fmt"
	pb "geecache/geecachepb"
	"geecache/singleflight"
	"log"
	"sync"
)

// Getter 会加载键对应的数据。
type Getter interface {
	Get(key string) ([]byte, error)
}

// GetterFunc 一个函数类型,让其他普通函数符合接口
type GetterFunc func(key string) ([]byte, error)

// 给Getterfunc一个Get方法以实现Getter接口
func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}

// Group 是一个缓存命名空间，其关联的数据被加载并分散存储在一组对等节点上。
type Group struct {
	name      string
	getter    Getter
	mainCache cache
	peers     PeerPicker
	// 使用 singleflight.Group 来确保每个键只被获取一次
	loader *singleflight.Group
}

// 管理所有 Group
var (
	mu     sync.RWMutex
	groups = make(map[string]*Group)
)

func NewGroup(name string, cacheBytes int64, getter Getter) *Group {
	if getter == nil {
		panic("nil Getter")
	}
	//保证并发创建 Group 时 map 不会被破坏
	mu.Lock()
	defer mu.Unlock()
	g := &Group{
		name:      name,
		getter:    getter,
		mainCache: cache{cacheBytes: cacheBytes},
		loader:    &singleflight.Group{},
	}
	//保存
	groups[name] = g
	return g
}

// GetGroup 返回先前使用 NewGroup 创建的指定名称的组，或者
// 如果不存在这样的组，则返回 nil。
func GetGroup(name string) *Group {
	mu.Lock()
	g := groups[name]
	mu.Unlock()

	return g
}

// 从缓存中获取指定键的值,Get方法
func (g *Group) Get(key string) (ByteView, error) {
	if key == "" {
		return ByteView{}, fmt.Errorf("key is required")
	}
	if v, ok := g.mainCache.get(key); ok {
		log.Println("[GeeCache] hit")
		return v, nil
	}
	return g.load(key)
}

// // RegisterPeers 注册一个 PeerPicker，用于选择远程对等节点
func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("RegisterPeerPicker called more than once")
	}
	g.peers = peers
}

// 获取失败，走回调load方法
func (g *Group) load(key string) (value ByteView, err error) {
	// 无论并发调用者的数量是多少,每个键只会被获取一次（无论是在本地还是远程）
	viewi, err := g.loader.Do(key, func() (interface{}, error) {
		// 如果已注册节点池（分布式模式）
		if g.peers != nil {
			if peer, ok := g.peers.PickPeer(key); ok {
				if value, err = g.getFromPeer(peer, key); err == nil {
					return value, nil
				}
				log.Println("[GeeCache] 从对等节点获取数据失败", err)
			}
		}
		//远程获取失败、key 归本地、或未注册节点池，走本地加载
		return g.getLocally(key)
	})
	if err == nil {
		return viewi.(ByteView), nil
	}
	return
}

// load底层getFromPeer方法：远程获取数据
func (g *Group) getFromPeer(peer PeerGetter, key string) (ByteView, error) {
	req := &pb.Request{
		Group: g.name,
		Key:   key,
	}
	res := &pb.Response{}
	err := peer.Get(req, res)
	if err != nil {
		return ByteView{}, err
	}
	return ByteView{b: res.Value}, nil
}

// load底层，getLocally
func (g *Group) getLocally(key string) (ByteView, error) {
	bytes, err := g.getter.Get(key)
	if err != nil {
		return ByteView{}, err
	}
	//复制一份,用来修改(保证一致性)
	value := ByteView{b: cloneBytes(bytes)}
	g.populateCache(key, value)
	return value, nil
}

// 写回缓存方法populateCache
func (g *Group) populateCache(key string, value ByteView) {
	g.mainCache.add(key, value)
}
