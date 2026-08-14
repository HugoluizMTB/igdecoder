package igdecoder

import (
	"errors"
	"testing"
)

func TestParseShortcode(t *testing.T) {
	cases := map[string]string{
		"https://www.instagram.com/reel/Db_MGIMx-Px/?utm_source=ig_web_copy_link": "Db_MGIMx-Px",
		"https://www.instagram.com/p/C8-WuWIy5lk/":                                "C8-WuWIy5lk",
		"https://instagram.com/reels/ABC123/":                                     "ABC123",
		"https://www.instagram.com/tv/XYZ789/":                                    "XYZ789",
		"Db_MGIMx-Px":                                                             "Db_MGIMx-Px",
	}
	for in, want := range cases {
		got, err := ParseShortcode(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%q -> %q, queria %q", in, got, want)
		}
	}
	if _, err := ParseShortcode("https://example.com/nada"); !errors.Is(err, ErrBadPermalink) {
		t.Error("url invalida deveria falhar")
	}
}

func TestShortcodeToMediaID(t *testing.T) {
	got, err := ShortcodeToMediaID("CylsWHNrUdB")
	if err != nil {
		t.Fatal(err)
	}
	if got != "3217172542446716737" {
		t.Errorf("CylsWHNrUdB -> %s, queria 3217172542446716737", got)
	}
	if _, err := ShortcodeToMediaID("!!!"); !errors.Is(err, ErrBadPermalink) {
		t.Error("shortcode invalido deveria falhar")
	}
}
