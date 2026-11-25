package media

import (
	"encoding/binary"
	"io"
)

// probeJPEG reads the frame size from the first SOF marker. It walks the
// marker segments at the start of the file and never reads image data.
func probeJPEG(r io.ReaderAt, info *Info) error {
	info.Container = "JPEG"
	info.Kind = Image
	info.Source = "jpeg"
	var off int64 = 2
	var hdr [4]byte
	for i := 0; i < 64; i++ {
		if _, err := r.ReadAt(hdr[:], off); err != nil {
			return nil
		}
		if hdr[0] != 0xFF {
			return nil
		}
		marker := hdr[1]
		if marker == 0xFF { // padding
			off++
			continue
		}
		if marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 { // standalone markers
			off += 2
			continue
		}
		size := int64(binary.BigEndian.Uint16(hdr[2:4]))
		if marker == 0xDA || marker == 0xD9 { // start of scan / end: no frame header before data
			return nil
		}
		isSOF := marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC
		if isSOF {
			var sof [7]byte
			if _, err := r.ReadAt(sof[:], off+4); err != nil {
				return nil
			}
			info.Height = int(binary.BigEndian.Uint16(sof[1:3]))
			info.Width = int(binary.BigEndian.Uint16(sof[3:5]))
			info.BitDepth = int(sof[0])
			return nil
		}
		if marker == 0xE1 && size > 8 { // APP1: EXIF, for the camera
			readEXIF(r, off+4, size-2, info)
		}
		off += 2 + size
	}
	return nil
}

// probePNG reads the IHDR chunk that every PNG starts with.
func probePNG(r io.ReaderAt, info *Info) error {
	info.Container = "PNG"
	info.Kind = Image
	info.Source = "png"
	var ihdr [21]byte
	if _, err := r.ReadAt(ihdr[:], 8); err != nil || string(ihdr[4:8]) != "IHDR" {
		return nil
	}
	info.Width = int(binary.BigEndian.Uint32(ihdr[8:12]))
	info.Height = int(binary.BigEndian.Uint32(ihdr[12:16]))
	info.BitDepth = int(ihdr[16])
	return nil
}

// probeTIFF reads the first image directory of a TIFF, which also covers
// DNG and most camera raw files (ARW, NEF, CR2). The first directory is
// usually the full image or its preview; either gives the frame size.
func probeTIFF(r io.ReaderAt, info *Info) error {
	info.Container = "TIFF"
	info.Kind = Image
	info.Source = "tiff"
	var hdr [8]byte
	if _, err := r.ReadAt(hdr[:], 0); err != nil {
		return nil
	}
	var bo binary.ByteOrder = binary.LittleEndian
	if string(hdr[:2]) == "MM" {
		bo = binary.BigEndian
	}
	readIFD(r, int64(bo.Uint32(hdr[4:8])), 0, bo, info)
	return nil
}

// readEXIF handles the APP1 segment of a JPEG: "Exif\0\0" then a TIFF
// structure whose offsets are relative to its own start.
func readEXIF(r io.ReaderAt, off, size int64, info *Info) {
	var head [14]byte
	if _, err := r.ReadAt(head[:], off); err != nil || string(head[:6]) != "Exif\x00\x00" {
		return
	}
	base := off + 6
	var bo binary.ByteOrder = binary.LittleEndian
	if string(head[6:8]) == "MM" {
		bo = binary.BigEndian
	}
	readIFD(r, int64(bo.Uint32(head[10:14])), base, bo, info)
}

// readIFD walks one TIFF directory for the size, camera and date tags.
func readIFD(r io.ReaderAt, ifd, base int64, bo binary.ByteOrder, info *Info) {
	var cnt [2]byte
	if _, err := r.ReadAt(cnt[:], base+ifd); err != nil {
		return
	}
	n := int(bo.Uint16(cnt[:]))
	if n > 512 {
		return
	}
	entries := make([]byte, n*12)
	if _, err := r.ReadAt(entries, base+ifd+2); err != nil {
		return
	}
	str := func(e []byte) string {
		count := int64(bo.Uint32(e[4:8]))
		if count <= 0 || count > 256 {
			return ""
		}
		buf := make([]byte, count)
		if count <= 4 {
			copy(buf, e[8:8+count])
		} else if _, err := r.ReadAt(buf, base+int64(bo.Uint32(e[8:12]))); err != nil {
			return ""
		}
		for len(buf) > 0 && (buf[len(buf)-1] == 0 || buf[len(buf)-1] == ' ') {
			buf = buf[:len(buf)-1]
		}
		return string(buf)
	}
	num := func(e []byte) int {
		switch bo.Uint16(e[2:4]) {
		case 3: // SHORT
			return int(bo.Uint16(e[8:10]))
		case 4: // LONG
			return int(bo.Uint32(e[8:12]))
		}
		return 0
	}
	var exifIFD int64
	var make, model string
	for i := 0; i < n; i++ {
		e := entries[i*12 : i*12+12]
		switch bo.Uint16(e[0:2]) {
		case 0x100: // ImageWidth
			if info.Width == 0 {
				info.Width = num(e)
			}
		case 0x101: // ImageLength
			if info.Height == 0 {
				info.Height = num(e)
			}
		case 0x10F: // Make
			make = str(e)
		case 0x110: // Model
			model = str(e)
		case 0x132: // DateTime
			if info.Created == "" {
				info.Created = str(e)
			}
		case 0x8769: // Exif IFD pointer
			exifIFD = int64(bo.Uint32(e[8:12]))
		}
	}
	if make != "" || model != "" {
		info.Camera = camera(make, model)
	}
	if exifIFD > 0 {
		readExifSub(r, exifIFD, base, bo, info)
	}
}

// readExifSub reads the Exif sub-directory for the pixel size and the
// original date, which cameras fill in more reliably than the main one.
func readExifSub(r io.ReaderAt, ifd, base int64, bo binary.ByteOrder, info *Info) {
	var cnt [2]byte
	if _, err := r.ReadAt(cnt[:], base+ifd); err != nil {
		return
	}
	n := int(bo.Uint16(cnt[:]))
	if n > 512 {
		return
	}
	entries := make([]byte, n*12)
	if _, err := r.ReadAt(entries, base+ifd+2); err != nil {
		return
	}
	for i := 0; i < n; i++ {
		e := entries[i*12 : i*12+12]
		v := 0
		switch bo.Uint16(e[2:4]) {
		case 3:
			v = int(bo.Uint16(e[8:10]))
		case 4:
			v = int(bo.Uint32(e[8:12]))
		}
		switch bo.Uint16(e[0:2]) {
		case 0xA002: // PixelXDimension
			if v > 0 {
				info.Width = v
			}
		case 0xA003: // PixelYDimension
			if v > 0 {
				info.Height = v
			}
		case 0x9003: // DateTimeOriginal
			count := int64(bo.Uint32(e[4:8]))
			if count > 4 && count <= 64 {
				buf := make([]byte, count)
				if _, err := r.ReadAt(buf, base+int64(bo.Uint32(e[8:12]))); err == nil {
					for len(buf) > 0 && buf[len(buf)-1] == 0 {
						buf = buf[:len(buf)-1]
					}
					info.Created = string(buf)
				}
			}
		}
	}
}

// camera joins a make and model without repeating the make ("Canon
// Canon EOS R5" becomes "Canon EOS R5").
func camera(make, model string) string {
	switch {
	case make == "":
		return model
	case model == "":
		return make
	case len(model) >= len(make) && equalFold(model[:len(make)], make):
		return model
	}
	return make + " " + model
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if x >= 'A' && x <= 'Z' {
			x += 'a' - 'A'
		}
		if y >= 'A' && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}
