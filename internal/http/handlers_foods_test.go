package httpserver

import "testing"

func TestClampPage(t *testing.T) {
	cases := []struct {
		raw            string
		n, page, count int
	}{
		{"", 0, 1, 1},      // empty result still has a page
		{"", 120, 1, 3},    // default first page
		{"2", 120, 2, 3},   // in range
		{"9", 120, 3, 3},   // past the end clamps to last
		{"0", 120, 1, 3},   // below range clamps to first
		{"abc", 120, 1, 3}, // garbage falls back to first
		{"2", 100, 2, 2},   // exact multiple of page size
	}
	for _, c := range cases {
		page, count := clampPage(c.raw, c.n, 50)
		if page != c.page || count != c.count {
			t.Errorf("clampPage(%q, %d) = %d, %d; want %d, %d", c.raw, c.n, page, count, c.page, c.count)
		}
	}
}
