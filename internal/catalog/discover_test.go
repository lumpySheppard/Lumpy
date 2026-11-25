package catalog

import "testing"

func TestCleanName(t *testing.T) {
	ig := map[string]bool{"BS": true}
	for in, want := range map[string][2]string{
		"04_BS_REDHAND":                  {"REDHAND", "REDHAND"},
		"03_BS_LATEAUGUST_2025-11-16_JJ": {"LATEAUGUST", "LATEAUGUST"},
		"01_PROJECTS/02_ICEMAN_EP2":      {"ICEMAN EP2", "ICEMAN"},
		"01_PROJECTS/FT_PILOT":           {"FT PILOT", "FT"},
		"07_CS-Teaser_v2":                {"CS Teaser v2", "CS"},
		"01_PROJECTS/BS":                 {"", ""},
	} {
		name, key := cleanName(in, ig)
		if name != want[0] || key != want[1] {
			t.Errorf("%s: got %q/%q, want %q/%q", in, name, key, want[0], want[1])
		}
	}
}

func TestLooksLikeProject(t *testing.T) {
	yes := []string{"04_BS_REDHAND", "01_PROJECTS/ICEMAN_EP1", "01_PROJECTS/02_FT_PILOT", "Projects/Closing Soon"}
	no := []string{"01_PROJECTS", "01_RAW", "09_OUTPUT", "01_1TB", "DCIM", "04_BS_REDHAND/01_RAW", "01_PROJECTS/ICEMAN/01_RAW"}
	for _, p := range yes {
		if !looksLikeProject(p) {
			t.Errorf("%s should look like a project", p)
		}
	}
	for _, p := range no {
		if looksLikeProject(p) {
			t.Errorf("%s should not look like a project", p)
		}
	}
}
