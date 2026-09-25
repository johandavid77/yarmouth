package version

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0", "1.0", 0},
		{"1.1", "1.0", 1},
		{"1.0", "1.1", -1},
		{"1.0~rc1", "1.0", -1},
		{"1.0~rc1", "1.0~rc2", -1},
		{"1.0~rc10", "1.0", -1},
		{"1.0", "1.0-1", -1},
		{"1.0-1", "1.0-2", -1},
		{"1.0-2", "1.0", 1},
		{"2:1.0", "1.2", 1},
		{"1.2", "2:1.0", -1},
		{"1.0.2", "1.0.10", -1},
		{"1.0.10", "1.0.2", 1},
		{"1.0-10", "1.0-10a", -1},
		{"1.0-10a", "1.0-10", 1},
		{"1:2.3", "1:2.3-1", -1},
		{"0.5.9", "0.5.10", -1},
		{"6.0", "6.0+dfsg", -1},
		{"1.0-1", "1.0-1", 0},
		{"1.0-rc1", "1.0", 1},
		{"1.0~~rc1", "1.0~rc1", -1},
		{"2.39", "2.4", 1},
	}
	for _, c := range cases {
		got, err := Compare(c.a, c.b)
		if err != nil {
			t.Fatalf("Compare(%q, %q): %v", c.a, c.b, err)
		}
		if got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestParse(t *testing.T) {
	bad := []string{"", ":1.0", "1:", ":"}
	for _, s := range bad {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q): se esperaba error", s)
		}
	}
	v, err := Parse("2:1.2.3-1")
	if err != nil {
		t.Fatal(err)
	}
	if v.Epoch != 2 || v.Upstream != "1.2.3" || v.Revision != "1" {
		t.Errorf("Parse inesperado: %+v", v)
	}
	if v.String() != "2:1.2.3-1" {
		t.Errorf("String = %q", v.String())
	}
}

func TestSorting(t *testing.T) {
	unsorted := []string{"1.0-1", "1.0-2", "1.0~rc1", "1.0", "1.1", "0.9"}
	veryUnsorted := append([]string(nil), unsorted...)
	for i := 1; i < len(veryUnsorted); i++ {
		for j := i; j > 0; j-- {
			lt, err := Less(veryUnsorted[j], veryUnsorted[j-1])
			if err != nil {
				t.Fatal(err)
			}
			if lt {
				veryUnsorted[j], veryUnsorted[j-1] = veryUnsorted[j-1], veryUnsorted[j]
			}
		}
	}
	want := []string{"0.9", "1.0~rc1", "1.0", "1.0-1", "1.0-2", "1.1"}
	for i := range want {
		if veryUnsorted[i] != want[i] {
			t.Fatalf("orden = %v, want %v", veryUnsorted, want)
		}
	}
}
