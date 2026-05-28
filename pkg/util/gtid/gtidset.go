package gtid

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type GTIDSet struct {
	segments []segment
	raw      string
}

type segment struct {
	uuid      string
	intervals [][]int64
}

func New(id string) (*GTIDSet, error) {
	return parse(id)
}

func parse(id string) (*GTIDSet, error) {
	if id == "" {
		return &GTIDSet{}, nil
	}

	segments := make([]segment, 0)
	id = canonicalize(id)

	for _, rawSeg := range strings.Split(id, ",") {
		rawSeg = strings.TrimSpace(rawSeg)
		parts := strings.SplitN(rawSeg, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid GTID segment %q", rawSeg)
		}

		uuid := strings.TrimSpace(parts[0])
		if uuid == "" {
			return nil, fmt.Errorf("invalid GTID segment %q: empty UUID", rawSeg)
		}

		intervals := make([][]int64, 0)
		for _, rawInterval := range strings.Split(parts[1], ":") {
			interval, ok := parseInterval(rawInterval)
			if ok {
				intervals = append(intervals, interval)
			}
		}
		if len(intervals) == 0 {
			return nil, fmt.Errorf("invalid GTID segment %q: no numeric intervals", rawSeg)
		}

		segments = append(segments, segment{uuid: uuid, intervals: intervals})
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("invalid GTID set %q", id)
	}

	return &GTIDSet{segments: segments, raw: id}, nil
}

func parseInterval(rawInterval string) ([]int64, bool) {
	rawInterval = strings.TrimSpace(rawInterval)
	if rawInterval == "" {
		return nil, false
	}

	parts := strings.Split(rawInterval, "-")
	if len(parts) > 2 {
		return nil, false
	}

	start, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return nil, false
	}

	end := start
	if len(parts) == 2 {
		end, err = strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			return nil, false
		}
	}
	if start > end {
		return nil, false
	}

	return []int64{start, end}, true
}

func (s *GTIDSet) IsEmpty() bool {
	return s == nil || s.raw == ""
}

func (s *GTIDSet) Start(uuidFilter string) (string, int64) {
	if s.IsEmpty() {
		return "", 0
	}

	if s == nil || len(s.segments) == 0 || len(s.segments[0].intervals) == 0 {
		return "", 0
	}

	start := s.segments[0].intervals[0][0]
	uuid := s.segments[0].uuid
	for _, seg := range s.segments {
		if uuidFilter != "" && seg.uuid != uuidFilter {
			continue
		}

		for _, interval := range seg.intervals {
			if interval[0] < start {
				start = interval[0]
				uuid = seg.uuid
			}
		}
	}

	return uuid, start
}

func (s *GTIDSet) End(uuidFilter string) (string, int64) {
	if s.IsEmpty() {
		return "", 0
	}

	if s == nil || len(s.segments) == 0 || len(s.segments[0].intervals) == 0 {
		return "", 0
	}

	end := s.segments[0].intervals[0][1]
	uuid := s.segments[0].uuid
	for _, seg := range s.segments {
		if uuidFilter != "" && seg.uuid != uuidFilter {
			continue
		}
		for _, interval := range seg.intervals {
			if interval[1] > end {
				end = interval[1]
				uuid = seg.uuid
			}
		}
	}

	return uuid, end
}

func (s *GTIDSet) ContainsSeq(uuid string, seq int64) bool {
	if s.IsEmpty() {
		return false
	}

	for _, seg := range s.segments {
		if seg.uuid != uuid {
			continue
		}

		for _, interval := range seg.intervals {
			if seq >= interval[0] && seq <= interval[1] {
				return true
			}
		}
	}
	return false
}

func (s *GTIDSet) ContainsUUID(uuid string) bool {
	if s.IsEmpty() {
		return false
	}

	for _, seg := range s.segments {
		if seg.uuid == uuid {
			return true
		}
	}
	return false
}

func (s *GTIDSet) Equal(other *GTIDSet) bool {
	var sRaw, otherRaw string
	if s != nil {
		sRaw = s.raw
	}
	if other != nil {
		otherRaw = other.raw
	}
	return sRaw == otherRaw
}

func (s *GTIDSet) String() string {
	return s.raw
}

func canonicalize(s string) string {
	parts := make([]string, 0)
	for segment := range strings.SplitSeq(s, ",") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		parts = append(parts, segment)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
