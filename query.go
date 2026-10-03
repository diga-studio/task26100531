package txwindow

// QueryBatch 批量计算特征，结果与 queries 一一对应。
// 每条查询独立：取每笔交易在序号不大于 K 时的最后修订，
// 未被撤回且交易时间在 [T-24h, T) 内的计入汇总。
func (s *Store) QueryBatch(queries []Query) []Feature {
	features := make([]Feature, len(queries))
	for i, q := range queries {
		features[i] = s.queryOne(q)
	}
	return features
}

func (s *Store) queryOne(q Query) Feature {
	var f Feature
	lower := q.SampleTimeMillis - WindowMillis
	for _, k := range s.byAccount[q.Account] {
		v, ok := lastVisible(s.histories[k], q.UpToSeq)
		if !ok || v.withdrawn {
			continue
		}
		if v.timeMillis < lower || v.timeMillis >= q.SampleTimeMillis {
			continue
		}
		f.Count++
		f.TotalCents += v.amountCents
	}
	return f
}

// lastVisible 返回序号不大于 upToSeq 的最后一个版本。
// 历史按 seq 升序存储，二分查找。
func lastVisible(hist []version, upToSeq uint64) (version, bool) {
	lo, hi := 0, len(hist)
	for lo < hi {
		mid := (lo + hi) / 2
		if hist[mid].seq <= upToSeq {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo == 0 {
		return version{}, false
	}
	return hist[lo-1], true
}
