# go-concurrent-skiplist-map

基于多层跳表（Skip List）的并发安全有序 Map，Go 实现，仅使用标准库，无任何第三方依赖。

## 特性

- string -> string 有序映射：key 非空且唯一，按字典序排列
- 节点层高由 key 的 FNV-1a 哈希确定性计算：同一 key 永远得到相同高度，无随机源、无种子配置
- 查找 / 插入 / 删除沿多层索引推进，平均 O(log n)；Range 为 O(log n + m)
- `sync.RWMutex` 保证并发安全：读读并发、读写互斥，无重复 key、无顺序破坏、无数据竞争
- Delete 会断开节点出现的全部层，不留悬空索引
- Close 后拒绝写入但保留读取，且 Close 幂等

## API

| 方法 | 说明 |
| --- | --- |
| `New() *Map` | 创建空 Map |
| `Put(key, value string) error` | 插入或更新；key 已存在时只更新 value，不新增节点 |
| `Get(key string) (string, bool)` | 查询 value |
| `Delete(key string) (bool, error)` | 删除并断开所有层；返回 key 是否存在 |
| `LowerBound(key string) (Entry, bool)` | 第一个 key >= 目标的条目；不存在返回 false |
| `Range(start, end string) ([]Entry, error)` | `[start, end)` 有序独立副本，要求 start < end |
| `Snapshot() []Entry` | 全部条目的字典序独立副本 |
| `Len() int` | 当前 key 数量 |
| `Close()` | 关闭写入（Put/Delete 返回 ErrClosed），读取仍可用，可重复调用 |

错误值：`ErrEmptyKey`（空 key）、`ErrClosed`（关闭后写入）、`ErrInvalidRange`（Range 参数非法）。

## 项目结构

- `skiplist.go` — 跳表核心库（包 `skiplistmap`）
- `skiplist_test.go` — 8 个自动化测试
- `cmd/skiplist-cli/main.go` — 命令行演示

## 运行

```sh
go test -race ./...        # 运行全部测试（含竞态检测）
go run ./cmd/skiplist-cli  # 运行演示：插入/更新/LowerBound/Range/Delete/Close 后读取
```

## 使用示例

```go
m := skiplistmap.New()
_ = m.Put("apple", "red fruit")
_ = m.Put("apple", "green fruit") // 只更新 value
v, ok := m.Get("apple")           // "green fruit", true
e, ok := m.LowerBound("app")      // {apple green fruit}, true
entries, _ := m.Range("a", "c")   // [a, c) 有序副本
deleted, _ := m.Delete("apple")   // true
n := m.Len()
snap := m.Snapshot()
m.Close()                         // 之后 Put/Delete 返回 ErrClosed，读取仍可用
```

