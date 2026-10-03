package txwindow

import "testing"

const hourMillis int64 = 60 * 60 * 1000

func mustIngest(t *testing.T, s *Store, revisions ...Revision) uint64 {
	t.Helper()
	seq, err := s.IngestBatch(revisions)
	if err != nil {
		t.Fatalf("IngestBatch 失败: %v", err)
	}
	return seq
}

func queryOne(t *testing.T, s *Store, account string, sampleTime int64, upToSeq uint64) Feature {
	t.Helper()
	features := s.QueryBatch([]Query{{Account: account, SampleTimeMillis: sampleTime, UpToSeq: upToSeq}})
	if len(features) != 1 {
		t.Fatalf("期望 1 条结果，得到 %d 条", len(features))
	}
	return features[0]
}

func expect(t *testing.T, got Feature, wantCount int, wantTotal int64) {
	t.Helper()
	if got.Count != wantCount || got.TotalCents != wantTotal {
		t.Errorf("期望 %d笔%d分，得到 %d笔%d分", wantCount, wantTotal, got.Count, got.TotalCents)
	}
}

// 场景1：同一笔交易的新增、修正、撤回、恢复在不同截止序号下的可见性。
func TestSingleTransactionLifecycle(t *testing.T) {
	s := New()
	const T = 1_000_000_000_000

	seq1 := mustIngest(t, s, Revision{Account: "a", EventID: "e1", TimeMillis: T - hourMillis, AmountCents: 100})
	seq2 := mustIngest(t, s, Revision{Account: "a", EventID: "e1", TimeMillis: T + hourMillis, AmountCents: 200})
	seq3 := mustIngest(t, s, Revision{Account: "a", EventID: "e1", Withdrawn: true})
	seq4 := mustIngest(t, s, Revision{Account: "a", EventID: "e1", TimeMillis: T - 2*hourMillis, AmountCents: 300})

	expect(t, queryOne(t, s, "a", T, seq1), 1, 100)
	expect(t, queryOne(t, s, "a", T, seq2), 0, 0)
	expect(t, queryOne(t, s, "a", T, seq3), 0, 0)
	expect(t, queryOne(t, s, "a", T, seq4), 1, 300)
}

// 场景2：晚到交易、窗口边界、同时间多笔交易，以及历史截止序号结果不可变。
func TestLateArrivalBoundariesAndStability(t *testing.T) {
	s := New()
	const T = 2_000_000_000_000

	// 批次1：窗口左端（计入）、右端（不计入）、同时间两笔不同事件。
	seq1 := mustIngest(t, s,
		Revision{Account: "a", EventID: "left", TimeMillis: T - WindowMillis, AmountCents: 10},
		Revision{Account: "a", EventID: "right", TimeMillis: T, AmountCents: 20},
		Revision{Account: "a", EventID: "same1", TimeMillis: T - 1000, AmountCents: 1},
		Revision{Account: "a", EventID: "same2", TimeMillis: T - 1000, AmountCents: 2},
	)

	// 批次2：一笔交易时间属于过去窗口，但晚到，只在 seq2 起可见。
	seq2 := mustIngest(t, s,
		Revision{Account: "a", EventID: "late", TimeMillis: T - hourMillis, AmountCents: 50},
	)

	// 晚到交易在录入前的截止序号下不可见；左端计入、右端不计入、同时间分别计数。
	expect(t, queryOne(t, s, "a", T, seq1), 3, 13)
	expect(t, queryOne(t, s, "a", T, seq2), 4, 63)

	// 穿插录入：撤回 left、修正 same1 金额、新增无关交易。
	mustIngest(t, s, Revision{Account: "a", EventID: "left", Withdrawn: true})
	mustIngest(t, s, Revision{Account: "a", EventID: "same1", TimeMillis: T - 1000, AmountCents: 99})
	mustIngest(t, s, Revision{Account: "b", EventID: "other", TimeMillis: T - 500, AmountCents: 7})

	// 以相同样本时间和历史截止序号查询，结果保持不变。
	expect(t, queryOne(t, s, "a", T, seq1), 3, 13)
	expect(t, queryOne(t, s, "a", T, seq2), 4, 63)
}

func TestDuplicateRevisionInBatchRejected(t *testing.T) {
	s := New()
	_, err := s.IngestBatch([]Revision{
		{Account: "a", EventID: "e1", TimeMillis: 1, AmountCents: 1},
		{Account: "a", EventID: "e1", TimeMillis: 2, AmountCents: 2},
	})
	if err == nil {
		t.Fatal("批内重复修订同一交易应返回错误")
	}
	// 整批不生效，也不消耗序号。
	seq, err := s.IngestBatch([]Revision{{Account: "a", EventID: "e1", TimeMillis: 1, AmountCents: 1}})
	if err != nil {
		t.Fatalf("IngestBatch 失败: %v", err)
	}
	if seq != 1 {
		t.Fatalf("失败的批次不应消耗序号，期望 1，得到 %d", seq)
	}
	expect(t, queryOne(t, s, "a", 10, seq), 1, 1)
}

func TestEmptyWindowAndBatchQuery(t *testing.T) {
	s := New()
	seq := mustIngest(t, s, Revision{Account: "a", EventID: "e1", TimeMillis: 5000, AmountCents: 42})

	features := s.QueryBatch([]Query{
		{Account: "missing", SampleTimeMillis: 10000, UpToSeq: seq}, // 无此账户
		{Account: "a", SampleTimeMillis: 1000, UpToSeq: seq},        // 窗口为空
		{Account: "a", SampleTimeMillis: 10000, UpToSeq: seq},       // 命中
		{Account: "a", SampleTimeMillis: 10000, UpToSeq: seq},       // 重复查询
	})
	if len(features) != 4 {
		t.Fatalf("期望 4 条结果，得到 %d 条", len(features))
	}
	expect(t, features[0], 0, 0)
	expect(t, features[1], 0, 0)
	expect(t, features[2], 1, 42)
	expect(t, features[3], 1, 42)
}
