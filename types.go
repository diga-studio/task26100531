// Package txwindow 提供时点正确的交易窗口特征计算。
// 数据仅保存在进程内存中，供上层训练样本构建器直接调用。
package txwindow

// WindowMillis 是窗口长度：24 小时，单位毫秒。
const WindowMillis int64 = 24 * 60 * 60 * 1000

// Revision 表示对一笔交易的一次修订。
// 交易由 (Account, EventID) 唯一标识。
// Withdrawn 为 true 表示撤回该交易；
// 否则表示新增或修正，需携带完整的交易时间与金额。
type Revision struct {
	Account     string
	EventID     string
	TimeMillis  int64 // UTC 毫秒
	AmountCents int64 // 金额，整数分
	Withdrawn   bool
}

// Query 描述一条特征查询。
type Query struct {
	Account          string
	SampleTimeMillis int64  // 样本时间 T，UTC 毫秒
	UpToSeq          uint64 // 截止可见序号 K（含）
}

// Feature 是单条查询的汇总结果。
type Feature struct {
	Count      int   // 窗口内交易笔数
	TotalCents int64 // 窗口内总金额，整数分
}

// Store 保存全部修订历史，支持批量录入与批量查询。
// 不是并发安全的；调用方需自行串行化。
type Store struct {
	nextSeq uint64
	// histories 按 (Account, EventID) 保存修订历史，按可见序号升序。
	histories map[txKey][]version
	// byAccount 索引账户下的全部交易键，加速查询。
	byAccount map[string][]txKey
}

type txKey struct {
	account string
	eventID string
}

type version struct {
	seq         uint64
	timeMillis  int64
	amountCents int64
	withdrawn   bool
}

// New 创建一个空的 Store。
func New() *Store {
	return &Store{
		nextSeq:   1,
		histories: make(map[txKey][]version),
		byAccount: make(map[string][]txKey),
	}
}
