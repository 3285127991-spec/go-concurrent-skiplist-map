# go-concurrent-skiplist-map

基于真正多层跳表（skip list）实现的并发有序 string map，纯 Go 标准库，无第三方依赖。

## 特性

- 唯一非空 string key，按字典序排列，`Put` 已存在 key 时仅更新 value
- 节点层高由 key 的 FNV-1a 哈希确定性计算（同一 key 永远同一高度），无随机种子
- `Put` / `Get` / `Delete` / `LowerBound` 复杂度 O(log n)，`Range` 为 O(log n + m)
- `Delete` 正确断开节点出现的全部层，不留悬空索引
- `sync.RWMutex` 保证多 goroutine 并发安全：无重复 key、无顺序破坏、无数据竞争
- `Close` 后拒绝写入（`Put` 返回 `ErrClosed`，`Delete` 为 no-op），读取仍可用，`Close` 可重复调用

## API

```go
m := skiplistmap.New()
err := m.Put("apple", "red fruit")   // 空 key 返回 ErrEmptyKey
v, ok := m.Get("apple")
ok = m.Delete("apple")                // 返回是否删除成功
e, ok := m.LowerBound("app")          // 第一个 key >= "app" 的条目
entries, err := m.Range("a", "c")     // [start, end)，要求 start < end
all := m.Snapshot()                   // 全量有序独立副本
n := m.Len()
err = m.Close()                       // 幂等；之后写拒绝、读可用
```

`Range` 与 `Snapshot` 返回的都是独立副本，修改返回值不影响 map。

## 运行测试

```sh
go test -race ./...
```

## 运行 CLI 演示

```sh
go run ./cmd/skipcli
```

演示插入、更新、LowerBound、Range、Delete 以及 Close 后的读取行为。

## 项目结构

- `skiplist.go` — 跳表 map 库实现（包 `skiplistmap`）
- `skiplist_test.go` — 8 个自动化测试（顺序、更新、LowerBound、Range、跨层 Delete、边界参数、Snapshot/Close、基础并发）
- `cmd/skipcli/main.go` — 命令行演示
