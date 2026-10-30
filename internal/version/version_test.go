package version

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.10.0", "1.9.9", 1},
		{"0.48", "0.48.0", 0},
		{"2.0.0-rc1", "2.0.0", 0},
		{"1.2", "1.10", -1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestExtract(t *testing.T) {
	v, ok := Extract("fzf 0.56.3 (brew)", `([\d.]+)`)
	if !ok || v != "0.56.3" {
		t.Fatalf("got %q %v", v, ok)
	}
	if _, ok := Extract("nothing", `(\d+\.\d+)`); ok {
		t.Fatal("want no match")
	}
}
