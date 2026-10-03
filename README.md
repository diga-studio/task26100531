# txwindow

时点正确的交易窗口特征库（纯进程内存，无外部依赖）。供上层训练样本构建器直接调用，
按「样本时间 T + 数据可见序号 K」双维度还原特征，避免将当时尚未知晓的信息泄漏到旧样本。

## 功能

- 按账户汇总过去 24 小时（`[T-86400000ms, T)`，UTC 毫秒）的交易笔数与总金额（整数分）。
- 批量录入修订（新增 / 修正时间或金额 / 撤回 / 恢复），每批获得一个全局递增可见序号。
- 批量查询：每条查询独立指定账户、样本时间 T 与截止序号 K；历史序号的结果永不因后续修订改变。

## 使用

```go
s := txwindow.New()

// 录入：每批复修订共享一个可见序号；批内同一 (账户, 事件ID) 最多修订一次。
seq1, _ := s.IngestBatch([]txwindow.Revision{
    {Account: "a", EventID: "e1", TimeMillis: t0, AmountCents: 100},
})
seq2, _ := s.IngestBatch([]txwindow.Revision{
    {Account: "a", EventID: "e1", Withdrawn: true}, // 撤回
})
seq3, _ := s.IngestBatch([]txwindow.Revision{
    {Account: "a", EventID: "e1", TimeMillis: t1, AmountCents: 300}, // 恢复（需完整时间与金额）
})

// 查询：晚到数据只在实际录入序号起可见，旧序号结果保持不变。
features := s.QueryBatch([]txwindow.Query{
    {Account: "a", SampleTimeMillis: T, UpToSeq: seq1},
    {Account: "a", SampleTimeMillis: T, UpToSeq: seq3},
})
// features[i] 为 txwindow.Feature{Count, TotalCents}
```

## 说明

- `Store` 非并发安全，调用方需自行串行化；录入与查询可任意交替。
- 空批次也会消耗一个可见序号；校验失败（批内重复修订）的批次不生效且不消耗序号。
- 查询截止序号 K 为「含」，即取序号 ≤ K 的最后修订。

## 测试

```sh
go test ./...
```
