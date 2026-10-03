// Package txnwindow 提供时点正确的交易窗口特征计算。
// 数据仅保存在进程内存中，供上层训练样本构建器直接调用。
package txnwindow

import (
	"sort"
	"sync"
)

// WindowMillis 是特征窗口长度：过去 24 小时，单位毫秒。
const WindowMillis int64 = 24 * 60 * 60 * 1000

// Revision 表示对一笔交易的一次修订。
// 同一批次内同一交易（账户+事件ID）最多出现一次。
type Revision struct {
	Account string // 账户标识
	EventID string // 事件标识，与账户一起唯一标识一笔交易
	Revoked bool   // true 表示撤回该交易
	TimeMs  int64  // 交易时间，UTC 整数毫秒；撤回时忽略
	Amount  int64  // 金额，整数分；撤回时忽略
}

// Query 表示一条特征查询。
type Query struct {
	Account string // 账户标识
	Sample  int64  // 样本时间 T，UTC 整数毫秒
	Cutoff  int64  // 截止可见序号 K，仅使用序号 <= K 的修订
}

// Feature 是一条查询的汇总结果。
type Feature struct {
	Count int   // 窗口内交易笔数
	Sum   int64 // 窗口内总金额，整数分
}

// version 是一笔交易在某个可见序号上的状态。
type version struct {
	seq     int64
	revoked bool
	timeMs  int64
	amount  int64
}

// account 保存一个账户下所有交易的修订历史。
type account struct {
	txns map[string][]version // 事件ID -> 按序号递增的修订序列
}

// Store 保存全部修订历史，支持批量录入与批量查询。
// 已返回的可见序号对应的查询结果不受后续录入影响。
type Store struct {
	mu       sync.RWMutex
	seq      int64
	accounts map[string]*account
}

// NewStore 创建一个空的 Store。
func NewStore() *Store {
	return &Store{accounts: make(map[string]*account)}
}

// Apply 录入一批修订，返回该批次的全局递增可见序号（从 1 开始）。
// 批内同一交易最多修订一次，所有修订在返回的序号一起可见。
// 空批次也会消耗一个序号，保证序号与已完成的批次一一对应。
func (s *Store) Apply(batch []Revision) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	seq := s.seq
	for _, r := range batch {
		acc := s.accounts[r.Account]
		if acc == nil {
			acc = &account{txns: make(map[string][]version)}
			s.accounts[r.Account] = acc
		}
		hist := acc.txns[r.EventID]
		// 批内重复修订同一交易时，以最后一次为准。
		if n := len(hist); n > 0 && hist[n-1].seq == seq {
			hist[n-1] = version{seq: seq, revoked: r.Revoked, timeMs: r.TimeMs, amount: r.Amount}
		} else {
			hist = append(hist, version{seq: seq, revoked: r.Revoked, timeMs: r.TimeMs, amount: r.Amount})
		}
		acc.txns[r.EventID] = hist
	}
	return seq
}

// Query 批量查询。每条查询独立使用其样本时间与截止序号，
// 结果不受后续录入影响。空窗口返回 0 笔、0 金额。
func (s *Store) Query(queries []Query) []Feature {
	s.mu.RLock()
	defer s.mu.RUnlock()

	features := make([]Feature, len(queries))
	for i, q := range queries {
		features[i] = s.queryOne(q)
	}
	return features
}

// queryOne 计算单条查询：先取每笔交易在序号 <= K 时的最后修订，
// 再按撤回状态与窗口 [T-WindowMillis, T) 过滤并汇总。
func (s *Store) queryOne(q Query) Feature {
	acc := s.accounts[q.Account]
	if acc == nil {
		return Feature{}
	}
	lo := q.Sample - WindowMillis
	var f Feature
	for _, hist := range acc.txns {
		// hist 按 seq 递增，取最后一个 seq <= K 的版本。
		idx := sort.Search(len(hist), func(i int) bool { return hist[i].seq > q.Cutoff }) - 1
		if idx < 0 {
			continue // 该交易在 K 之前尚不可见（晚到数据不回填）
		}
		v := hist[idx]
		if v.revoked || v.timeMs < lo || v.timeMs >= q.Sample {
			continue
		}
		f.Count++
		f.Sum += v.amount
	}
	return f
}
