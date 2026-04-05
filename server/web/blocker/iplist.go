package blocker

import (
	"bytes"
	"fmt"
	"net"
	"sort"
)

type Ranger interface {
	Lookup(net.IP) (r Range, ok bool)
	NumRanges() int
}

type IPList struct {
	ranges []Range
}

type Range struct {
	First, Last net.IP
	Description string
}

func (r Range) String() string {
	return fmt.Sprintf("%s-%s: %s", r.First, r.Last, r.Description)
}

func New(initSorted []Range) *IPList {
	if len(initSorted) == 0 {
		return &IPList{}
	}

	ranges := make([]Range, len(initSorted))
	copy(ranges, initSorted)
	sort.Slice(ranges, func(i, j int) bool {
		return ipLess(ranges[i].First, ranges[j].First)
	})

	return &IPList{
		ranges: ranges,
	}
}

func ipLess(a, b net.IP) bool {
	la := len(a)
	lb := len(b)

	if la != lb {
		return la < lb
	}

	return bytes.Compare(a, b) < 0
}

func (ipl *IPList) NumRanges() int {
	if ipl == nil {
		return 0
	}

	return len(ipl.ranges)
}

func (ipl *IPList) Lookup(ip net.IP) (r Range, ok bool) {
	if ipl == nil || len(ipl.ranges) == 0 {
		return
	}

	return ipl.lookupBinary(ip)
}

func (ipl *IPList) lookupBinary(ip net.IP) (Range, bool) {
	ranges := ipl.ranges
	lo, hi := 0, len(ranges)-1

	for lo <= hi {
		mid := lo + (hi-lo)/2
		r := ranges[mid]

		if ipLess(ip, r.First) {
			hi = mid - 1
		} else if ipLess(r.Last, ip) {
			lo = mid + 1
		} else {
			return r, true
		}
	}

	return Range{}, false
}

func minifyIP(ip *net.IP) {
	v4 := ip.To4()
	if v4 != nil {
		*ip = append(make([]byte, 0, 4), v4...)
	}
}
