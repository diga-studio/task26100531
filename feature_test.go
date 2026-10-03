package txnwindow

import "testing"

const hourMs int64 = 60 * 60 * 1000

// 场景1：单笔交易依次经历 新增 -> 修正 -> 撤回 -> 恢复，
// 固定样本时间 T，四个截止序号的结果依次为
// {1,100} {0,0} {0,0} {1,300}。
func TestSingleTransactionLifecycle(t *testing.T) {
	s := NewStore()
	const T int64 = 1_700_000_000_000
	const acc, ev = "a1", "e1"

	k1 := s.Apply([]Revision{{Account: acc, EventID: ev, TimeMs: T - hourMs, Amount: 100}})
	k2 := s.Apply([]Revision{{Account: acc, EventID: ev, TimeMs: T + hourMs, Amount: 200}})
	k3 := s.Apply([]Revision{{Account: acc, EventID: ev, Revoked: true}})
	k4 := s.Apply([]Revision{{Account: acc, EventID: ev, TimeMs: T - 2*hourMs, Amount: 300}})

	got := s.Query([]Query{
		{Account: acc, Sample: T, Cutoff: k1},
		{Account: acc, Sample: T, Cutoff: k2},
		{Account: acc, Sample: T, Cutoff: k3},
		{Account: acc, Sample: T, Cutoff: k4},
	})
	want := []Feature{{1, 100}, {0, 0}, {0, 0}, {1, 300}}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cutoff k%d: got %+v, want %+v", i+1, got[i], want[i])
		}
	}
}

// 场景2：晚到交易、窗口两端边界、同时间多笔交易，
// 以及历史截止序号的结果不受后续录入影响。
func TestLateArrivalBoundariesAndStability(t *testing.T) {
	s := NewStore()
	const T int64 = 1_700_000_000_000
	const acc = "a1"

	// 批次1：窗口左端（计入）、右端（不计入）、同时间两笔（分别计数并累加）。
	k1 := s.Apply([]Revision{
		{Account: acc, EventID: "left", TimeMs: T - WindowMillis, Amount: 10},
		{Account: acc, EventID: "right", TimeMs: T, Amount: 20},
		{Account: acc, EventID: "same1", TimeMs: T - hourMs, Amount: 100},
		{Account: acc, EventID: "same2", TimeMs: T - hourMs, Amount: 250},
	})

	// 批次2：晚到交易，交易时间在窗口内，但录入序号更晚。
	k2 := s.Apply([]Revision{
		{Account: acc, EventID: "late", TimeMs: T - 2*hourMs, Amount: 500},
	})

	// 晚到交易在录入前的截止序号下不计入；窗口左端计入、右端不计入；
	// 同时间的不同事件分别计数并累加金额。
	got := s.Query([]Query{
		{Account: acc, Sample: T, Cutoff: k1},
		{Account: acc, Sample: T, Cutoff: k2},
	})
	want := []Feature{{3, 360}, {4, 860}}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("query %d: got %+v, want %+v", i, got[i], want[i])
		}
	}

	// 穿插录入：撤回 same1、修正 same2 金额、新增一笔。
	s.Apply([]Revision{
		{Account: acc, EventID: "same1", Revoked: true},
		{Account: acc, EventID: "same2", TimeMs: T - hourMs, Amount: 999},
		{Account: acc, EventID: "extra", TimeMs: T - 3*hourMs, Amount: 7},
	})

	// 以相同样本时间和历史截止序号查询，结果保持不变。
	again := s.Query([]Query{
		{Account: acc, Sample: T, Cutoff: k1},
		{Account: acc, Sample: T, Cutoff: k2},
	})
	for i := range want {
		if again[i] != want[i] {
			t.Errorf("historical query %d changed: got %+v, want %+v", i, again[i], want[i])
		}
	}
}

// 空窗口与未知账户返回 0 笔、0 金额；不同事件ID即使时间金额相同也分别计数。
func TestEmptyAndDistinctEvents(t *testing.T) {
	s := NewStore()
	const T int64 = 1_700_000_000_000

	k := s.Apply([]Revision{
		{Account: "a1", EventID: "e1", TimeMs: T - hourMs, Amount: 50},
		{Account: "a1", EventID: "e2", TimeMs: T - hourMs, Amount: 50},
	})

	got := s.Query([]Query{
		{Account: "unknown", Sample: T, Cutoff: k},             // 未知账户
		{Account: "a1", Sample: T - 2*WindowMillis, Cutoff: k}, // 空窗口
		{Account: "a1", Sample: T, Cutoff: k},                  // 同时间同金额的不同事件
	})
	want := []Feature{{0, 0}, {0, 0}, {2, 100}}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("query %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}
