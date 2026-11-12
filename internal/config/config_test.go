package config

import "testing"

func TestParseHouseholdThemeURL(t *testing.T) {
	cases := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{raw: "", want: ""},
		{raw: " https://home.example/api/theme ", want: "https://home.example/api/theme"},
		{raw: "http://household-app:8000/api/theme", want: "http://household-app:8000/api/theme"},
		{raw: "ftp://home.example/theme", wantErr: true},
		{raw: "/api/theme", wantErr: true},
	}
	for _, tc := range cases {
		got, err := parseHouseholdThemeURL(tc.raw)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("parseHouseholdThemeURL(%q) = %q, %v", tc.raw, got, err)
		}
	}
}
