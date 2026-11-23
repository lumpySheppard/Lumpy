package media

import (
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"io"
	"math"
	"os"
)

// DPX, the frame format DaVinci Resolve writes its gallery stills in
// (and film scans come as): a fixed header, then one frame of RGB,
// usually 10-bit packed three to a 32-bit word. Lumpy reads the common
// forms (8, 10, 12 and 16-bit RGB and RGBA, and luma) so a still can
// be looked at without opening Resolve.

// DecodeDPX reads one DPX frame as an 8-bit image, down-sampling to at
// most maxWidth pixels wide while reading so a 4K still costs little.
func DecodeDPX(path string, maxWidth int) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var hdr [2048]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return nil, errors.New("not a DPX file")
	}
	var bo binary.ByteOrder
	switch string(hdr[0:4]) {
	case "SDPX":
		bo = binary.BigEndian
	case "XPDS":
		bo = binary.LittleEndian
	default:
		return nil, errors.New("not a DPX file")
	}
	dataOff := bo.Uint32(hdr[4:8])
	orient := bo.Uint16(hdr[768:770])
	elements := bo.Uint16(hdr[770:772])
	w := int(bo.Uint32(hdr[772:776]))
	h := int(bo.Uint32(hdr[776:780]))
	if elements < 1 || w <= 0 || h <= 0 || w > 32768 || h > 32768 {
		return nil, errors.New("DPX header makes no sense")
	}
	// Image element 0 starts at 780: data sign (4), low/high data and
	// quantity (16), descriptor (1), transfer (1), colorimetric (1),
	// bit size (1), packing (2), encoding (2), offset (4).
	descriptor := hdr[800]
	transfer := hdr[801]
	bits := int(hdr[803])
	packing := bo.Uint16(hdr[804:806])
	encoding := bo.Uint16(hdr[806:808])
	if off := bo.Uint32(hdr[808:812]); off != 0 && off != 0xffffffff {
		dataOff = off
	}
	if encoding != 0 {
		return nil, errors.New("run-length DPX is not supported")
	}
	var comps int
	switch descriptor {
	case 50:
		comps = 3
	case 51:
		comps = 4
	case 6, 1, 2, 3, 4:
		comps = 1
	default:
		return nil, errors.New("unsupported DPX colour layout")
	}
	step := 1
	for w/step > maxWidth {
		step++
	}
	ow, oh := (w+step-1)/step, (h+step-1)/step
	img := image.NewRGBA(image.Rect(0, 0, ow, oh))
	// Bytes per line, as the file lays them out.
	var lineBytes int
	switch {
	case bits == 8:
		lineBytes = w * comps
	case bits == 16:
		lineBytes = w * comps * 2
	case bits == 10 && packing != 0:
		lineBytes = ((w*comps + 2) / 3) * 4 // three samples per 32-bit word
	case bits == 12 && packing != 0:
		lineBytes = w * comps * 2 // one sample per 16-bit word
	case bits == 10 || bits == 12:
		lineBytes = (w*comps*bits + 31) / 32 * 4 // tightly packed, word-aligned per line
	default:
		return nil, errors.New("unsupported DPX bit depth")
	}
	lineBytes = (lineBytes + 3) &^ 3
	maxVal := float64(int(1)<<bits - 1)
	// Log (Cineon) stills are flat as stored; a simple curve brings
	// them back to something like a picture.
	toByte := func(v float64) uint8 {
		n := v / maxVal
		if transfer == 3 {
			n = math.Pow(10, (n-0.95)*2.048*2) * 1.0
			n = math.Pow(n, 1/2.2)
		}
		if n < 0 {
			n = 0
		}
		if n > 1 {
			n = 1
		}
		return uint8(n*255 + 0.5)
	}
	line := make([]byte, lineBytes)
	sample := make([]float64, comps)
	for y := 0; y < h; y += step {
		if _, err := f.Seek(int64(dataOff)+int64(y)*int64(lineBytes), io.SeekStart); err != nil {
			return nil, err
		}
		if _, err := io.ReadFull(f, line); err != nil {
			return nil, errors.New("DPX file is cut short")
		}
		for x := 0; x < w; x += step {
			for c := 0; c < comps; c++ {
				i := x*comps + c
				switch {
				case bits == 8:
					sample[c] = float64(line[i])
				case bits == 16:
					sample[c] = float64(bo.Uint16(line[i*2:]))
				case bits == 10 && packing != 0:
					word := bo.Uint32(line[(i/3)*4:])
					shift := uint(22 - (i%3)*10)
					if packing == 2 { // method B: padding in the high bits
						shift = uint((i % 3) * 10)
					}
					sample[c] = float64((word >> shift) & 0x3ff)
				case bits == 12 && packing != 0:
					v := bo.Uint16(line[i*2:])
					if packing == 1 {
						v >>= 4
					} else {
						v &= 0xfff
					}
					sample[c] = float64(v)
				default: // tightly packed
					bit := i * bits
					word := bo.Uint32(line[(bit/32)*4:])
					within := uint(bit % 32)
					v := (word >> within) & (1<<uint(bits) - 1)
					if within+uint(bits) > 32 {
						next := bo.Uint32(line[(bit/32+1)*4:])
						v |= (next << (32 - within)) & (1<<uint(bits) - 1)
					}
					sample[c] = float64(v)
				}
			}
			var px color.RGBA
			if comps >= 3 {
				px = color.RGBA{toByte(sample[0]), toByte(sample[1]), toByte(sample[2]), 255}
			} else {
				g := toByte(sample[0])
				px = color.RGBA{g, g, g, 255}
			}
			ox, oy := x/step, y/step
			if orient&1 != 0 { // right to left
				ox = ow - 1 - ox
			}
			if orient&2 != 0 { // bottom to top
				oy = oh - 1 - oy
			}
			img.SetRGBA(ox, oy, px)
		}
	}
	return img, nil
}
