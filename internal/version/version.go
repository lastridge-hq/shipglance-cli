// Package version compares CLI versions for the update notice. It accepts
// MAJOR.MINOR.PATCH[-PRERELEASE] with an optional "v"; anything else (a local
// "dev" build) does not parse and is never compared.
package version

import (
	"strconv"
	"strings"
)

type Version struct {
	nums  [3]int
	pre   string
	valid bool
}

func Parse(raw string) (Version, bool) {
	value, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(raw), "v"), "+")
	core, pre, hasPre := strings.Cut(value, "-")
	if hasPre && pre == "" {
		return Version{}, false
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	var v Version
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || part == "" || (len(part) > 1 && part[0] == '0') {
			return Version{}, false
		}
		v.nums[i] = n
	}
	v.pre, v.valid = pre, true
	return v, true
}

func (v Version) Valid() bool { return v.valid }

func (v Version) String() string {
	if !v.valid {
		return ""
	}
	s := strconv.Itoa(v.nums[0]) + "." + strconv.Itoa(v.nums[1]) + "." + strconv.Itoa(v.nums[2])
	if v.pre != "" {
		s += "-" + v.pre
	}
	return s
}

// Less reports whether v precedes other. Pre-releases sort before their
// release; pre-release identifiers compare as plain strings, which is enough
// to decide "is a newer release out".
func (v Version) Less(other Version) bool {
	for i := range v.nums {
		if v.nums[i] != other.nums[i] {
			return v.nums[i] < other.nums[i]
		}
	}
	switch {
	case v.pre == other.pre:
		return false
	case v.pre == "":
		return false
	case other.pre == "":
		return true
	default:
		return v.pre < other.pre
	}
}
