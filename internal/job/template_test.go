package job

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateFromTemplate(t *testing.T) {
	tmp := t.TempDir()
	pht := filepath.Join(tmp, "Video Editing Template.pht")
	files := filepath.Join(pht, "Files")
	for _, d := range []string{"01_Original footage/Online", "01_Original footage/Offline", "03_Project files/[Client] Premiere", "04_Graphics Sources", "09_OUTPUT/Final"} {
		os.MkdirAll(filepath.Join(files, d), 0o755)
	}
	os.WriteFile(filepath.Join(files, "01_Original footage", "Description.txt"), []byte("Project: [Project Name] for {client}\nEditor: %Editor%\n"), 0o644)
	os.WriteFile(filepath.Join(files, "04_Graphics Sources", "Countdown.mov"), []byte("binary [Project Name] stays"), 0o644)
	os.WriteFile(filepath.Join(files, "[ProjectNumber]_notes.txt"), []byte("n"), 0o644)
	os.WriteFile(filepath.Join(files, ".DS_Store"), []byte("x"), 0o644)

	src, err := TemplateFiles(pht)
	if err != nil || src != files {
		t.Fatalf("TemplateFiles: %q %v", src, err)
	}
	params := TemplateParams{"Project Number": "16", "Client": "BS", "Project Name": "ARIA", "Date": "2026-10-14", "Editor": "studioshepp"}
	if got := Fill("[Project Number]_[Client]_[Project Name]_[Date]_[Editor]", params); got != "16_BS_ARIA_2026-10-14_studioshepp" {
		t.Errorf("Fill: %q", got)
	}
	dest := filepath.Join(tmp, "RAID", "16_BS_ARIA_2026-10-14_studioshepp")
	n, _, err := CreateFromTemplate(context.Background(), src, dest, params, NewReportTo(io.Discard))
	if err != nil || n != 3 {
		t.Fatalf("create: %d %v", n, err)
	}
	for _, p := range []string{"01_Original footage/Online", "01_Original footage/Offline", "03_Project files/BS Premiere", "09_OUTPUT/Final", "16_notes.txt", "04_Graphics Sources/Countdown.mov"} {
		if _, err := os.Stat(filepath.Join(dest, p)); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "01_Original footage", "Description.txt")); string(b) != "Project: ARIA for BS\nEditor: studioshepp\n" {
		t.Errorf("text: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "04_Graphics Sources", "Countdown.mov")); string(b) != "binary [Project Name] stays" {
		t.Errorf("binary changed: %q", b)
	}
	if _, err := os.Stat(filepath.Join(dest, ".DS_Store")); err == nil {
		t.Error(".DS_Store copied")
	}
	if _, _, err := CreateFromTemplate(context.Background(), src, dest, params, NewReportTo(io.Discard)); err == nil {
		t.Error("overwrote an existing folder")
	}
	if _, err := TemplateFiles(filepath.Join(tmp, "nothing.pht")); err == nil {
		t.Error("missing template accepted")
	}
}
