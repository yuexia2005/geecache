# GeeCache

一个用 Go 语言实现的分布式缓存系统，支持 HTTP 节点间通信、一致性哈希负载均衡、LRU 淘汰策略、singleflight 防击穿，以及 Protobuf 序列化。

## 架构

```
             curl "http://localhost:8888/api?key=Tom"
                         │
                         ▼
              ┌──────────────────────┐
              │     API 网关          │
              │   (port 8888)         │
              └──────────┬───────────┘
                         │ 一致性哈希选择节点
          ┌──────────────┼──────────────┐
          ▼              ▼              ▼
   ┌──────────┐  ┌──────────┐  ┌──────────┐
   │  Node-1  │  │  Node-2  │  │  Node-3  │
   │  :9001   │  │  :9002   │  │  :9003   │
   │ (Cache)  │  │ (Cache)  │  │ (Cache)  │
   └──────────┘  └──────────┘  └──────────┘
          │              │              │
          └──────────────┼──────────────┘
                         │ Protobuf HTTP 通信
                         ▼
              ┌──────────────────────┐
              │    一致性哈希环        │
              │  每节点 50 虚拟副本     │
              └──────────────────────┘
                         │
                         ▼
              ┌──────────────────────┐
              │    LRU 缓存淘汰        │
              │  singleflight 防击穿   │
              └──────────────────────┘
```

## 特性

| 特性 | 实现 |
|------|------|
| **LRU 淘汰** | 双向链表 + HashMap，O(1) 读写，支持容量限制及淘汰回调 |
| **一致性哈希** | 虚拟节点 + CRC32，节点增删时最小化数据迁移 |
| **分布式通信** | HTTP + Protobuf 序列化，节点间自动路由 |
| **缓存击穿防护** | singleflight 合并并发请求，同一 key 只回源一次 |
| **可扩展接口** | `PeerPicker` / `PeerGetter`，替换 gRPC 等协议只需实现两个接口 |

## 项目结构

```
geecache/
│                       # ═══ 项目配置 ═══
├── go.mod              #   模块定义 (module geecache)
├── go.sum
├── .gitignore
├── run.sh              #   一键启动脚本 (Linux/macOS)
├── README.md
│
│                       # ═══ 入口 ═══
├── cmd/
│   └── gee/
│       └── main.go     #   启动缓存节点 / API 服务
│
│                       # ═══ geecache 主包 (库代码) ═══
├── byteview.go         #   不可变字节视图
├── cache.go            #   并发安全缓存封装
├── geecache.go         #   核心：Group.Get / load / getFromPeer
├── http.go             #   HTTP 服务端 + PeerGetter 客户端
├── peers.go            #   PeerPicker / PeerGetter 接口定义
├── geecache_test.go    #   单元测试（含 Protobuf 通信测试）
│
│                       # ═══ 子包 ═══
├── consistenthash/     #   一致性哈希
│   ├── consistenthash.go
│   └── consistenthash_test.go
│
├── lru/                #   LRU 淘汰算法
│   ├── lru.go
│   └── lru_test.go
│
├── singleflight/       #   请求合并防击穿
│   ├── singleflight.go
│   └── singleflight_test.go
│
└── geecachepb/         #   Protobuf 定义
    ├── geecachepb.proto
    └── geecachepb.pb.go
```

## 快速开始

**环境要求**：Go 1.13+

### 编译 & 启动

**Linux / macOS：**

```bash
# 一键启动（3 个节点 + 测试）
bash run.sh

# 或手动分终端：
go build -o server ./cmd/gee
./server -port=8001        # 终端 1
./server -port=8002        # 终端 2
./server -port=8003 -api=1 # 终端 3
```

**Windows PowerShell：**

```powershell
go build -o server.exe ./cmd/gee

# 三个终端分别运行：
.\server.exe -port=8001
.\server.exe -port=8002
.\server.exe -port=8003 -api=1
```

### 测试

```bash
# 直接访问缓存节点
curl "http://localhost:9001/_geecache/scores/Tom"   # 630

# 通过 API 服务
curl "http://localhost:8888/api?key=Tom"   # 630
curl "http://localhost:8888/api?key=Jack"  # 589
curl "http://localhost:8888/api?key=Sam"   # 567
```

## 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-port` | `8001` | 缓存节点端口（8001 / 8002 / 8003） |
| `-api` | `false` | 设为 `1` 时同时启动 API 服务（端口 8888） |

## 运行测试

```bash
go test -v ./...
```

```
=== RUN   TestProtobufPeer
    ... Protobuf peer communication OK: got value=630
--- PASS: TestProtobufPeer (0.01s)
=== RUN   TestProtobufSerialize
    ... Protobuf serialize/deserialize OK: 13 bytes
--- PASS: TestProtobufSerialize (0.00s)
PASS
ok      geecache                (13 个测试全部通过)
```

## 核心模块

### geecache 主包

| 文件 | 职责 |
|------|------|
| `geecache.go` | `Group` 核心逻辑 — `Get` 查缓存 → `load` 回源 → `getFromPeer` 远程获取 → `getLocally` 本地加载 |
| `cache.go` | 并发安全缓存封装，延迟初始化 LRU |
| `byteview.go` | 不可变字节视图，保证读安全 |
| `http.go` | HTTP 服务端（响应缓存查询）+ `httpGetter` 客户端（向其他节点请求数据） |
| `peers.go` | `PeerPicker` / `PeerGetter` 接口——替换通信协议的唯一接入点 |

### lru/ — LRU 缓存淘汰

- 双向链表 + HashMap，Get / Add 均为 O(1)
- 支持最大容量限制和淘汰回调 `OnEvicted`

### consistenthash/ — 一致性哈希

- 基于 CRC32，每个节点 50 个虚拟副本
- 二分查找定位节点，O(log n)
- 节点增删时仅影响相邻节点数据

### singleflight/ — 防缓存击穿

- 同一 key 的并发请求只执行**一次**回调
- 其他等待者共享结果，避免大量请求同时穿透到后端

### geecachepb/ — Protobuf 通信

```protobuf
message Request  { string group = 1; string key = 2; }
message Response { bytes  value = 1; }

service GroupCache {
  rpc Get(Request) returns (Response);
}
```

节点间 HTTP 通信使用 Protobuf 序列化，相比 JSON 体积减少约 50%，速度提升约 5-10 倍。

重新生成 Go 代码：

```bash
protoc --go_out=. --go_opt=paths=source_relative geecachepb/geecachepb.proto
```

## 可扩展接口

```go
type PeerPicker interface {
    PickPeer(key string) (peer PeerGetter, ok bool)
}

type PeerGetter interface {
    Get(in *pb.Request, out *pb.Response) error
}
```

替换 `HTTPPool` 为 gRPC 或其他协议只需实现这两个接口即可。

## 技术栈

| 层次 | 技术 |
|------|------|
| 语言 | Go |
| 序列化 | Protocol Buffers v3 |
| 通信 | HTTP |
| 哈希 | CRC32 |
| 缓存淘汰 | LRU (双向链表 + HashMap) |
| 负载均衡 | 一致性哈希 (虚拟节点) |
