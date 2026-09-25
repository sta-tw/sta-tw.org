package profile

import "testing"

func TestInputNormalize(t *testing.T) {
	t.Run("trims and defaults link label to host", func(t *testing.T) {
		in := Input{
			DisplayName: "  Nova  ",
			Bio:         "  hello  ",
			Links: []Link{
				{Label: "  ", URL: "  https://example.test/path  "},
				{Label: "blog", URL: "http://blog.example.test"},
				{Label: "skip", URL: "   "},
			},
		}
		if err := in.Normalize(); err != nil {
			t.Fatalf("Normalize() = %v", err)
		}
		if in.DisplayName != "Nova" || in.Bio != "hello" {
			t.Fatalf("trim failed: %+v", in)
		}
		if len(in.Links) != 2 {
			t.Fatalf("links = %+v, want 2", in.Links)
		}
		if in.Links[0].Label != "example.test" || in.Links[0].URL != "https://example.test/path" {
			t.Fatalf("link[0] = %+v", in.Links[0])
		}
	})

	t.Run("rejects bad input", func(t *testing.T) {
		cases := map[string]Input{
			"display too long": {DisplayName: string(make([]rune, maxDisplayName+1))},
			"bio too long":     {Bio: string(make([]rune, maxBio+1))},
			"too many links":   {Links: make([]Link, maxLinks+1)},
			"non-http scheme":  {Links: []Link{{URL: "ftp://example.test"}}},
			"no host":          {Links: []Link{{URL: "https:///path"}}},
			"label too long":   {Links: []Link{{Label: string(make([]rune, maxLinkLabel+1)), URL: "https://example.test"}}},
		}
		for name, in := range cases {
			in := in
			if name == "too many links" {
				for i := range in.Links {
					in.Links[i] = Link{URL: "https://example.test"}
				}
			}
			if err := in.Normalize(); err == nil {
				t.Fatalf("%s: expected an error", name)
			}
		}
	})
}
