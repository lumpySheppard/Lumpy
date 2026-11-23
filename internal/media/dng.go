package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"os"
)

// DNG previews. A DNG is a TIFF whose raw mosaic needs a real raw
// developer, but nearly every DNG carries a JPEG preview of the
// developed picture in one of its image directories (Adobe's and
// Blackmagic's do). Lumpy finds that JPEG and uses it.

// DecodeDNGPreview returns the embedded preview of a DNG (or any TIFF
// that has one), the largest found.
func DecodeDNGPreview(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var head [8]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return nil, errors.New("not a DNG file")
	}
	var bo binary.ByteOrder
	switch string(head[0:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return nil, errors.New("not a DNG file")
	}
	if bo.Uint16(head[2:4]) != 42 {
		return nil, errors.New("not a DNG file")
	}
	type ifdInfo struct {
		compression, photometric uint32
		offsets, counts          []uint32
		width, height            uint32
	}
	var best *ifdInfo
	seen := map[uint32]bool{}
	var queue []uint32
	queue = append(queue, bo.Uint32(head[4:8]))
	readU := func(typ uint16, data []byte) uint32 {
		switch typ {
		case 3:
			return uint32(bo.Uint16(data))
		case 4, 13:
			return bo.Uint32(data)
		case 1:
			return uint32(data[0])
		}
		return 0
	}
	typeSize := map[uint16]int{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 7: 1, 9: 4, 10: 8, 13: 4}
	for len(queue) > 0 && len(seen) < 64 {
		off := queue[0]
		queue = queue[1:]
		if off == 0 || seen[off] {
			continue
		}
		seen[off] = true
		if _, err := f.Seek(int64(off), io.SeekStart); err != nil {
			continue
		}
		var nb [2]byte
		if _, err := io.ReadFull(f, nb[:]); err != nil {
			continue
		}
		n := int(bo.Uint16(nb[:]))
		if n > 4096 {
			continue
		}
		ents := make([]byte, n*12+4)
		if _, err := io.ReadFull(f, ents); err != nil {
			continue
		}
		info := &ifdInfo{}
		for i := 0; i < n; i++ {
			e := ents[i*12 : i*12+12]
			tag, typ, count := bo.Uint16(e[0:2]), bo.Uint16(e[2:4]), bo.Uint32(e[4:8])
			size := typeSize[typ]
			if size == 0 {
				continue
			}
			var data []byte
			if int(count)*size <= 4 {
				data = e[8:12]
			} else {
				if count > 1<<20 {
					continue
				}
				data = make([]byte, int(count)*size)
				pos, _ := f.Seek(0, io.SeekCurrent)
				f.Seek(int64(bo.Uint32(e[8:12])), io.SeekStart)
				io.ReadFull(f, data)
				f.Seek(pos, io.SeekStart)
			}
			vals := func() []uint32 {
				out := make([]uint32, count)
				for j := range out {
					out[j] = readU(typ, data[j*size:])
				}
				return out
			}
			switch tag {
			case 0x0100:
				info.width = readU(typ, data)
			case 0x0101:
				info.height = readU(typ, data)
			case 0x0103:
				info.compression = readU(typ, data)
			case 0x0106:
				info.photometric = readU(typ, data)
			case 0x0111, 0x0201: // strip offsets, or the JPEG interchange format offset
				info.offsets = vals()
			case 0x0117, 0x0202:
				info.counts = vals()
			case 0x014a: // SubIFDs
				queue = append(queue, vals()...)
			}
		}
		// A JPEG-compressed directory that isn't the raw mosaic (which
		// is photometric 32803 or 34892) is a preview.
		if (info.compression == 7 || info.compression == 6) && info.photometric != 32803 && info.photometric != 34892 && len(info.offsets) == 1 && len(info.counts) == 1 {
			if best == nil || info.width > best.width {
				best = info
			}
		}
		queue = append(queue, bo.Uint32(ents[n*12:n*12+4])) // the next IFD in the chain
	}
	if best == nil {
		return nil, errors.New("this DNG carries no preview")
	}
	if best.counts[0] > 64<<20 {
		return nil, errors.New("preview too large")
	}
	buf := make([]byte, best.counts[0])
	if _, err := f.Seek(int64(best.offsets[0]), io.SeekStart); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, errors.New("DNG file is cut short")
	}
	img, err := jpeg.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, errors.New("the DNG's preview could not be read")
	}
	return img, nil
}
