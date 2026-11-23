package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lumpysheppard/lumpy/internal/catalog"
)

func TestTouchLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	if err := touch(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, awakeFile)); !os.IsNotExist(err) {
		t.Fatalf("awake file left behind: %v", err)
	}
	ro := filepath.Join(dir, "ro")
	os.Mkdir(ro, 0o500)
	if os.Geteuid() != 0 {
		if err := touch(ro); err == nil {
			t.Fatal("touching a read-only folder should fail, not be silent")
		}
	}
}

func TestKeepAwakeRules(t *testing.T) {
	hdd := &catalog.Drive{}
	if !keepAwake(hdd) {
		t.Fatal("a plain hard drive is kept awake")
	}
	for name, d := range map[string]*catalog.Drive{
		"ssd":       {SolidState: true},
		"card icon": {Labels: catalog.Labels{Icon: "card"}},
		"ssd icon":  {Labels: catalog.Labels{Icon: "ssd"}},
		"let sleep": {Labels: catalog.Labels{LetSleep: true}},
		"never log": {Labels: catalog.Labels{NeverLog: true}},
	} {
		if keepAwake(d) {
			t.Errorf("%s should be left alone", name)
		}
	}
	raid := &catalog.Drive{Labels: catalog.Labels{Icon: "raid"}}
	if !keepAwake(raid) {
		t.Fatal("a RAID is kept awake")
	}
}

func TestIsCard(t *testing.T) {
	dir := t.TempDir()
	if isCard(dir, "USB") {
		t.Fatal("an empty drive is not a card")
	}
	if !isCard(dir, "Secure Digital") {
		t.Fatal("an SD reader's bus marks a card")
	}
	os.Mkdir(filepath.Join(dir, "DCIM"), 0o755)
	if !isCard(dir, "USB") {
		t.Fatal("a DCIM folder at the root marks a card")
	}
}
