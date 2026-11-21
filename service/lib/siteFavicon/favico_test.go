package siteFavicon

import (
	"net/url"
	"testing"
)

func TestFindIconURL(t *testing.T) {
	base, err := url.Parse("https://example.com/app/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ href, want string }{
		{"logo.png?v=3", "https://example.com/app/logo.png?v=3"},
		{"/images/logo.png", "https://example.com/images/logo.png"},
		{"//example.org/icon.svg", "https://example.org/icon.svg"},
	} {
		page := []byte(`<html><head><link rel="shortcut ICON" href="` + test.href + `"></head></html>`)
		got, err := findIconURL(base, page)
		if err != nil || got != test.want {
			t.Errorf("findIconURL for %q = %q, %v, want %q", test.href, got, err, test.want)
		}
	}
}
