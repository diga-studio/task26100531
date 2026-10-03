package txwindow

import "fmt"

// IngestBatch 录入一批修订，返回该批次获得的全局递增可见序号。
// 批内同一交易最多修订一次，否则返回错误且整批不生效。
// 整批修订在同一可见序号一起生效；空批次也会消耗一个序号。
func (s *Store) IngestBatch(revisions []Revision) (uint64, error) {
	seen := make(map[txKey]struct{}, len(revisions))
	for _, r := range revisions {
		k := txKey{account: r.Account, eventID: r.EventID}
		if _, dup := seen[k]; dup {
			return 0, fmt.Errorf("txwindow: 批次内交易 (%q, %q) 被重复修订", r.Account, r.EventID)
		}
		seen[k] = struct{}{}
	}

	seq := s.nextSeq
	s.nextSeq++

	for _, r := range revisions {
		k := txKey{account: r.Account, eventID: r.EventID}
		if _, ok := s.histories[k]; !ok {
			s.byAccount[r.Account] = append(s.byAccount[r.Account], k)
		}
		s.histories[k] = append(s.histories[k], version{
			seq:         seq,
			timeMillis:  r.TimeMillis,
			amountCents: r.AmountCents,
			withdrawn:   r.Withdrawn,
		})
	}
	return seq, nil
}
