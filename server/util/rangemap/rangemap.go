package rangemap

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/buildbuddy-io/buildbuddy/server/util/log"
)

var RangeOverlapError = errors.New("Range overlap")

// Ranges are [inclusive,exclusive)
type Range struct {
	Left  []byte
	Right []byte

	Val interface{}
}

func (r *Range) String() string {
	return fmt.Sprintf("[%s, %s)", string(r.Left), string(r.Right))
}

func (r *Range) Contains(key []byte) bool {
	// bytes.Compare(a,b) does:
	//  0 if a==b, -1 if a < b, and +1 if a > b
	greaterThanOrEqualToLeft := bytes.Compare(key, r.Left) > -1
	lessThanRight := bytes.Compare(key, r.Right) == -1

	contained := greaterThanOrEqualToLeft && lessThanRight
	log.Printf("Checking if %s contains %q returned %t", r, string(key), contained)
	return contained
}

type RangeMap struct {
	ranges []*Range
}

func New() *RangeMap {
	return &RangeMap{
		ranges: make([]*Range, 0),
	}
}

func (rm *RangeMap) Add(left, right []byte, value interface{}) error {
	i := sort.Search(len(rm.ranges), func(i int) bool {
		//  0 if a==b, -1 if a < b, and +1 if a > b
		c := bytes.Compare(rm.ranges[i].Left, right)
		b := c >= 0
		return b
	})

	insertIndex := i

	newRange := &Range{
		Left:  left,
		Right: right,
		Val:   value,
	}

	// if we're inserting anywhere but the very beginning, ensure that
	// we don't overlap with the range before us.
	prevRangeIndex := insertIndex - 1
	if len(rm.ranges) > 0 && prevRangeIndex >= 0 {
		if bytes.Compare(rm.ranges[prevRangeIndex].Right, left) > 0 {
			return RangeOverlapError
		}
	}

	if insertIndex >= len(rm.ranges) {
		rm.ranges = append(rm.ranges, newRange)
	} else {
		rm.ranges = append(rm.ranges[:insertIndex+1], rm.ranges[insertIndex:]...)
		rm.ranges[insertIndex] = newRange
	}
	log.Printf("Added new range: %s", newRange)
	return nil
}

func (rm *RangeMap) Remove(left, right []byte) {
	deleteIndex := -1
	for i, r := range rm.ranges {
		if bytes.Compare(left, r.Left) == 0 && bytes.Compare(right, r.Right) == 0 {
			deleteIndex = i
			break
		}
	}
	if deleteIndex != -1 {
		rm.ranges = append(rm.ranges[:deleteIndex], rm.ranges[deleteIndex+1:]...)
	}
}

func (rm *RangeMap) Get(key []byte) interface{} {
	log.Printf("Get called for %q, %s", string(key), rm)
	if len(rm.ranges) == 0 {
		return nil
	}

	// Search returns the smallest i for which func returns true.
	// We want the smallest range that is bigger than this key
	// aka, starts AFTER this key, and then we'll go one left of it
	i := sort.Search(len(rm.ranges), func(i int) bool {
		//  0 if a==b, -1 if a < b, and +1 if a > b
		return bytes.Compare(rm.ranges[i].Left, key) > 0
	})

	// This is safe anyway because of how sort.Search works, but
	// be clear so readers see this won't hit an out of range panic.
	if i > 0 {
		i -= 1
	}
	if rm.ranges[i].Contains(key) {
		return rm.ranges[i].Val
	}

	return nil

}

func (rm *RangeMap) String() string {
	buf := "RangeMap:\n"
	for i, r := range rm.ranges {
		buf += r.String()
		if i != len(rm.ranges)-1 {
			buf += "\n"
		}
	}
	return buf
}

func (rm *RangeMap) Ranges() []*Range {
	return rm.ranges
}

func (rm *RangeMap) Clear() {
	rm.ranges = make([]*Range, 0)
}