package provider

import "testing"

func TestAPIURLPreservesConfiguredPrefix(t *testing.T) {
	for _, test := range []struct{ base, want string }{
		{"https://host.invalid", "https://host.invalid/v1/images/generations"},
		{"https://host.invalid/", "https://host.invalid/v1/images/generations"},
		{"https://host.invalid/v1", "https://host.invalid/v1/images/generations"},
		{"https://host.invalid/v1/", "https://host.invalid/v1/images/generations"},
		{"https://host.invalid/custom/api/", "https://host.invalid/custom/api/images/generations"},
		{"https://host.invalid/custom/api?version=1", "https://host.invalid/custom/api/images/generations?version=1"},
	} {
		if got := APIURL(test.base, "/v1/images/generations"); got != test.want {
			t.Errorf("base %s: got %s, want %s", test.base, got, test.want)
		}
	}
}
