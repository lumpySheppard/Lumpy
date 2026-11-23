package media

import (
	"encoding/binary"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// A 4x2 frame of 10-bit RGB packed three to a word, big-endian.
func writeTestDPX(t *testing.T) string {
	t.Helper()
	hdr := make([]byte, 2048)
	copy(hdr[0:4], "SDPX")
	binary.BigEndian.PutUint32(hdr[4:], 2048)
	binary.BigEndian.PutUint16(hdr[770:], 1)
	binary.BigEndian.PutUint32(hdr[772:], 4)
	binary.BigEndian.PutUint32(hdr[776:], 2)
	hdr[800], hdr[801], hdr[803] = 50, 2, 10
	binary.BigEndian.PutUint16(hdr[804:], 1)
	binary.BigEndian.PutUint32(hdr[808:], 2048)
	px := [][3]uint32{{1023, 0, 0}, {0, 1023, 0}, {0, 0, 1023}, {512, 512, 512}}
	var body []byte
	for y := 0; y < 2; y++ {
		for _, p := range px {
			var w [4]byte
			binary.BigEndian.PutUint32(w[:], p[0]<<22|p[1]<<12|p[2]<<2)
			body = append(body, w[:]...)
		}
	}
	path := filepath.Join(t.TempDir(), "still.dpx")
	if err := os.WriteFile(path, append(hdr, body...), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDecodeDPX(t *testing.T) {
	img, err := DecodeDPX(writeTestDPX(t), 100)
	if err != nil {
		t.Fatal(err)
	}
	want := []color.RGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {128, 128, 128, 255}}
	for x, c := range want {
		if got := img.At(x, 1).(color.RGBA); got != c {
			t.Errorf("pixel %d: got %v want %v", x, got, c)
		}
	}
	if KindOf("a.dpx") != Image {
		t.Fatal("DPX is an image")
	}
}
