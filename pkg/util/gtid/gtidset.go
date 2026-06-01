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
	id = canonicalize(id)
	if id == "" {
		return &GTIDSet{}, nil
	}

	segments := make([]segment, 0)
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

type SegmentFilter func(seg segment) bool

func MatchesUUID(uuid string) SegmentFilter {
	return func(seg segment) bool {
		return seg.uuid == uuid
	}
}

func (s *GTIDSet) Start(filters ...SegmentFilter) (string, int64) {
	return s.selectSeq(
		func(interval []int64) int64 { return interval[0] },
		func(candidate, selected int64) bool { return candidate < selected },
		filters...,
	)
}

func (s *GTIDSet) End(filters ...SegmentFilter) (string, int64) {
	return s.selectSeq(
		func(interval []int64) int64 { return interval[1] },
		func(candidate, selected int64) bool { return candidate > selected },
		filters...,
	)
}

func (s *GTIDSet) selectSeq(seq func([]int64) int64, prefer func(candidate, selected int64) bool, filters ...SegmentFilter) (string, int64) {
	if s.IsEmpty() {
		return "", 0
	}

	var selected int64
	var uuid string
	found := false

outer:
	for _, seg := range s.segments {
		for _, filter := range filters {
			if !filter(seg) {
				continue outer
			}
		}

		for _, interval := range seg.intervals {
			candidate := seq(interval)
			if !found || prefer(candidate, selected) {
				selected = candidate
				uuid = seg.uuid
				found = true
			}
		}
	}
	if !found {
		return "", 0
	}

	return uuid, selected
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
