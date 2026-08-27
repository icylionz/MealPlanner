package weburl

import "testing"

func TestParseHTTP(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://recipes.example/soup?q=one#step", true},
		{"http://recipes.example:80/soup", true},
		{"javascript:alert(1)", false},
		{"//recipes.example/soup", false},
		{"https://user:pass@recipes.example/soup", false},
		{"https://recipes.example/soup\nnext", false},
		{"https://recipes.example/soup%0anext", false},
		{" https://recipes.example/soup", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := IsHTTP(tt.url); got != tt.want {
				t.Fatalf("IsHTTP(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}
