package geecache

import (
	"fmt"
	"geecache/consistenthash"
	pb "geecache/geecachepb"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"
)

const (
	defaultBasePath = "/_geecache/"
	defaultReplicas = 50
)

// HTTPPool 为一组 HTTP 节点实现了 PeerPicker。
type HTTPPool struct {
	//当前节点的完整 URL，例如 "https://example.net:8000"
	self       string
	basePath   string
	mu         sync.Mutex          //保护peer和 httpGetter
	peers      *consistenthash.Map // 一致性哈希环实例
	httpGetter map[string]*httpGetter
}

// NewHTTPPool 初始化一个 HTTP 对等节点池
func NewHTTPPool(self string) *HTTPPool {
	return &HTTPPool{
		self:     self,
		basePath: defaultBasePath,
	}
}

// 记录包含服务器名称的信息,统一日志格式
func (p *HTTPPool) Log(format string, v ...interface{}) {
	log.Printf("[Server %s] %s", p.self, fmt.Sprintf(format, v...))
}

// ServeHTTP 处理所有 HTTP 请求
func (p *HTTPPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//请求路径对比
	if !strings.HasPrefix(r.URL.Path, p.basePath) {
		panic("HTTPPool serving unexpected path: " + r.URL.Path)
	}
	p.Log("%s %s", r.Method, r.URL.Path)

	//解析 URL，提取 group 名和 key
	parts := strings.SplitN(r.URL.Path[len(p.basePath):], "/", 2)
	if len(parts) != 2 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	groupName := parts[0]
	key := parts[1]

	//查找缓存组
	group := GetGroup(groupName)
	if group == nil {
		http.Error(w, "no such group:"+groupName, http.StatusNotFound)
		return
	}
	//从 Group 中获取缓存值
	view, err := group.Get(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 将值作为 proto 消息写入响应正文。
	body, err := proto.Marshal(&pb.Response{Value: view.ByteSlice()})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	//返回缓存数据
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(body)

}

// Set 会更新池中的节点列表
func (p *HTTPPool) Set(peers ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	//创建一个全新的哈希环
	p.peers = consistenthash.New(defaultReplicas, nil)
	p.peers.Add(peers...)
	p.httpGetter = make(map[string]*httpGetter, len(peers))
	for _, peer := range peers {
		p.httpGetter[peer] = &httpGetter{baseURL: peer + p.basePath}
	}

}

// PickPeer 根据键选择一个对等节点
func (p *HTTPPool) PickPeer(key string) (PeerGetter, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if peer := p.peers.Get(key); peer != "" && peer != p.self {
		p.Log("Pick peer %s", peer)
		return p.httpGetter[peer], true
	}
	return nil, false
}

var _ PeerPicker = (*HTTPPool)(nil)

// 存储远程节点的基础 URL
type httpGetter struct {
	baseURL string
}

func (h *httpGetter) Get(in *pb.Request, out *pb.Response) error {
	//构造请求 URL
	u := fmt.Sprintf(
		"%v%v/%v",
		h.baseURL,
		url.QueryEscape(in.GetGroup()),
		url.QueryEscape(in.GetKey()),
	)
	//发送 GET 请求
	res, err := http.Get(u)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned: %v", res.Status)
	}

	//读取响应体
	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %v", err)
	}

	//反序列化 Protobuf 响应
	if err := proto.Unmarshal(bytes, out); err != nil {
		return fmt.Errorf("decoding response body: %v", err)
	}
	return nil
}

// 验证是否符合PeerGetter接口
var _ PeerGetter = (*httpGetter)(nil)
