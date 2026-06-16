package geecache

import (
	"fmt"
	pb "geecache/geecachepb"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestGetter(t *testing.T) {
	var f Getter = GetterFunc(func(key string) ([]byte, error) {
		return []byte(key), nil
	})
	expect := []byte("key")
	if v, _ := f.Get("key"); !reflect.DeepEqual(v, expect) {
		t.Errorf("callback failed")
	}
}

var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

func TestGet(t *testing.T) {
	loadCounts := make(map[string]int, len(db))
	gee := NewGroup("scores", 2<<10, GetterFunc(func(key string) ([]byte, error) {
		log.Println("[SlowDB] search key", key)
		if v, ok := db[key]; ok {
			if _, ok := loadCounts[key]; !ok {
				loadCounts[key] = 0
			}
			loadCounts[key] += 1
			return []byte(v), nil
		}
		return nil, fmt.Errorf("%s not exist", key)
	}))
	for k, v := range db {
		if view, err := gee.Get(k); err != nil || view.String() != v {
			t.Fatal("failed to get value of Tom")
		}
		if _, err := gee.Get(k); err != nil || loadCounts[k] > 1 {
			t.Fatalf("cache %s miss", k)
		}
	}
	if view, err := gee.Get("unknown"); err == nil {
		t.Fatalf("the value of unknow should be empty, but %s got", view)
	}
}

func TestGetGroup(t *testing.T) {
	groupName := "scores"
	NewGroup(groupName, 2<<10, GetterFunc(
		func(key string) (bytes []byte, err error) { return }))
	if group := GetGroup(groupName); group == nil || group.name != groupName {
		t.Fatalf("group %s not exist", groupName)
	}
	if group := GetGroup(groupName + "111"); group != nil {
		t.Fatalf("expect nil, but %s got", group.name)
	}
}

// TestProtobufPeer 验证节点间 Protobuf 通信（端到端）
func TestProtobufPeer(t *testing.T) {
	delete(groups, "scores") // 避免和 TestGet 创建的组冲突

	// 1. 创建模拟远程节点，返回 Protobuf 编码的数据
	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &pb.Response{Value: []byte("630")}
		body, _ := proto.Marshal(resp)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(body)
	}))
	defer remoteServer.Close()

	// 2. 创建节点池，注册远程节点（Set 自动拼接 basePath "/_geecache/"）
	pool := NewHTTPPool("http://localhost:8001")
	pool.Set(remoteServer.URL)

	// 3. 创建缓存组并注册 PeerPicker
	group := NewGroup("scores", 2<<10, GetterFunc(
		func(key string) ([]byte, error) {
			t.Fatal("local getter should not be called when peer returns data")
			return nil, nil
		}))
	group.RegisterPeers(pool)

	// 4. 通过 Protobuf 远程获取数据
	view, err := group.Get("Tom")
	if err != nil {
		t.Fatalf("getFromPeer via protobuf failed: %v", err)
	}
	if view.String() != "630" {
		t.Fatalf("expected 630, got %s", view.String())
	}
	t.Logf("Protobuf peer communication OK: got value=%s", view.String())
}

// TestProtobufSerialize 验证 Protobuf 序列化/反序列化
func TestProtobufSerialize(t *testing.T) {
	req := &pb.Request{Group: "scores", Key: "Tom"}
	data, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	req2 := &pb.Request{}
	if err := proto.Unmarshal(data, req2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if req2.GetGroup() != "scores" || req2.GetKey() != "Tom" {
		t.Fatalf("roundtrip mismatch: group=%s, key=%s", req2.GetGroup(), req2.GetKey())
	}
	t.Logf("Protobuf serialize/deserialize OK: %d bytes", len(data))
}
