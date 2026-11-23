package space

import "testing"

func TestJudge(t *testing.T) {
	cases := []struct {
		rel, home string
		dir       bool
		want      string
	}{
		{"System/Library", "Users/will", true, "critical"},
		{"Library/Caches/com.apple.x", "Users/will", true, "safe"},
		{"Applications/Final Cut Pro.app", "Users/will", true, "check"},
		{"Users/will/Library/Caches/Google", "Users/will", true, "safe"},
		{"Users/will/Library/Keychains", "Users/will", true, "critical"},
		{"Users/will/Library/Application Support/Slack/Cache", "Users/will", true, "safe"},
		{"Users/will/Library/Application Support/DaVinci Resolve", "Users/will", true, "critical"},
		{"Users/will/Documents/Scripts", "Users/will", true, "check"},
		{"Users/will/Pictures/Photos Library.photoslibrary", "Users/will", true, "critical"},
		{"Library/Caches", ".", true, "safe"},
		{"Library/Mail/V10", ".", true, "critical"},
		{".Trash", ".", true, "safe"},
		{"Movies/CacheClip", ".", true, "safe"},
		{"Movies/Cut.fcpbundle", ".", true, "check"},
		{"Downloads/Installer.dmg", ".", false, "check"},
		{"Day_01/A001.braw", "", false, ""},
		{"project/node_modules", "", true, "safe"},
	}
	for _, c := range cases {
		if got := Judge(c.rel, c.home, c.dir).Level; got != c.want {
			t.Errorf("%s (home %q): got %q, want %q", c.rel, c.home, got, c.want)
		}
	}
}
