package version

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Epoch    uint64
	Upstream string
	Revision string
	Raw      string
}

func digits(c byte) bool { return c >= '0' && c <= '9' }

func alpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func order(c byte) int {
	switch {
	case c == '~':
		return -1
	case c == 0 || digits(c):
		return 0
	case alpha(c):
		return int(c)
	default:
		return int(c) + 256
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

func verrevcmp(a, b string) int {
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		firstDiff := 0
		for (i < len(a) && !digits(a[i])) || (j < len(b) && !digits(b[j])) {
			var ac, bc byte
			if i < len(a) {
				ac = a[i]
			}
			if j < len(b) {
				bc = b[j]
			}
			if d := order(ac) - order(bc); d != 0 {
				return sign(d)
			}
			i++
			j++
		}
		for i < len(a) && a[i] == '0' {
			i++
		}
		for j < len(b) && b[j] == '0' {
			j++
		}
		for i < len(a) && j < len(b) && digits(a[i]) && digits(b[j]) {
			if firstDiff == 0 {
				firstDiff = int(a[i]) - int(b[j])
			}
			i++
			j++
		}
		if i < len(a) && digits(a[i]) {
			return 1
		}
		if j < len(b) && digits(b[j]) {
			return -1
		}
		if firstDiff != 0 {
			return sign(firstDiff)
		}
	}
	return 0
}

func Parse(s string) (Version, error) {
	if s == "" {
		return Version{}, fmt.Errorf("version vacia")
	}
	v := Version{Raw: s}
	rest := s
	if c := strings.IndexByte(s, ':'); c >= 0 {
		ep, err := strconv.ParseUint(s[:c], 10, 64)
		if err != nil {
			return Version{}, fmt.Errorf("epoch invalido %q: %w", s[:c], err)
		}
		v.Epoch = ep
		rest = s[c+1:]
	}
	if i := strings.LastIndexByte(rest, '-'); i >= 0 {
		v.Upstream = rest[:i]
		v.Revision = rest[i+1:]
	} else {
		v.Upstream = rest
	}
	if v.Upstream == "" {
		return Version{}, fmt.Errorf("falta version upstream en %q", s)
	}
	return v, nil
}

func CompareParsed(a, b Version) int {
	switch {
	case a.Epoch > b.Epoch:
		return 1
	case a.Epoch < b.Epoch:
		return -1
	}
	if d := verrevcmp(a.Upstream, b.Upstream); d != 0 {
		return d
	}
	return verrevcmp(a.Revision, b.Revision)
}

func Compare(a, b string) (int, error) {
	av, err := Parse(a)
	if err != nil {
		return 0, err
	}
	bv, err := Parse(b)
	if err != nil {
		return 0, err
	}
	return CompareParsed(av, bv), nil
}

func Less(a, b string) (bool, error) {
	c, err := Compare(a, b)
	if err != nil {
		return false, err
	}
	return c < 0, nil
}

func (v Version) String() string {
	var b strings.Builder
	if v.Epoch != 0 {
		fmt.Fprintf(&b, "%d:", v.Epoch)
	}
	b.WriteString(v.Upstream)
	if v.Revision != "" {
		b.WriteByte('-')
		b.WriteString(v.Revision)
	}
	return b.String()
}
