package siteFavicon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
)

var errInvalidICO = errors.New("invalid ico file")

// icoEntry 是 ICO 文件里的一帧
type icoEntry struct {
	width  int
	height int
	bpp    int
	size   int
	offset int
}

// isICO 判断内容是否是 ICO/CUR 文件
func isICO(data []byte) bool {
	return len(data) >= 6 &&
		data[0] == 0 && data[1] == 0 &&
		(data[2] == 1 || data[2] == 2) && data[3] == 0
}

// decodeICO 解析 ICO 文件，返回其中尺寸最大、位深最高的一帧。
// 绝大多数网站图标是 .ico，而 Go 标准库不支持该格式，这里自行解析。
func decodeICO(data []byte) (image.Image, error) {
	if !isICO(data) {
		return nil, errInvalidICO
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count <= 0 || len(data) < 6+count*16 {
		return nil, errInvalidICO
	}

	entries := make([]icoEntry, 0, count)
	for i := 0; i < count; i++ {
		p := data[6+i*16 : 6+i*16+16]
		w := int(p[0])
		if w == 0 {
			w = 256
		}
		h := int(p[1])
		if h == 0 {
			h = 256
		}
		bpp := int(binary.LittleEndian.Uint16(p[6:8]))
		size := int(binary.LittleEndian.Uint32(p[8:12]))
		offset := int(binary.LittleEndian.Uint32(p[12:16]))
		if size <= 0 || offset < 0 || offset+size > len(data) {
			continue
		}
		entries = append(entries, icoEntry{width: w, height: h, bpp: bpp, size: size, offset: offset})
	}
	if len(entries) == 0 {
		return nil, errInvalidICO
	}

	best := entries[0]
	for _, e := range entries[1:] {
		area, bestArea := e.width*e.height, best.width*best.height
		if area > bestArea || (area == bestArea && e.bpp > best.bpp) {
			best = e
		}
	}

	raw := data[best.offset : best.offset+best.size]
	// 部分现代图标在 ICO 里直接塞了 PNG
	if len(raw) > 8 && bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")) {
		return png.Decode(bytes.NewReader(raw))
	}
	return decodeICODIB(raw, best)
}

// decodeICODIB 解析 ICO 内部未压缩的 DIB 数据
func decodeICODIB(raw []byte, e icoEntry) (image.Image, error) {
	if len(raw) < 40 {
		return nil, errInvalidICO
	}
	headerSize := int(binary.LittleEndian.Uint32(raw[0:4]))
	if headerSize < 40 || headerSize > len(raw) {
		return nil, errInvalidICO
	}
	width := int(int32(binary.LittleEndian.Uint32(raw[4:8])))
	height2 := int(int32(binary.LittleEndian.Uint32(raw[8:12])))
	bpp := int(binary.LittleEndian.Uint16(raw[14:16]))
	compression := int(binary.LittleEndian.Uint32(raw[16:20]))
	colorsUsed := int(binary.LittleEndian.Uint32(raw[32:36]))

	if compression != 0 {
		return nil, errors.New("compressed ico frames are not supported")
	}
	// DIB 里的高度包含 1bpp 的 AND 掩码，实际高度要减半
	height := height2 / 2
	if height <= 0 {
		height = height2
	}
	if width <= 0 || height <= 0 || width > 1024 || height > 1024 {
		return nil, errInvalidICO
	}
	switch bpp {
	case 1, 4, 8, 24, 32:
	default:
		return nil, errors.New("unsupported ico bit depth")
	}

	// 调色板
	var palette []color.NRGBA
	if bpp <= 8 {
		n := colorsUsed
		if n == 0 {
			n = 1 << uint(bpp)
		}
		offset := headerSize
		if n > (len(raw)-offset)/4 {
			n = (len(raw) - offset) / 4
		}
		palette = make([]color.NRGBA, n)
		for i := 0; i < n; i++ {
			q := raw[offset+i*4:]
			palette[i] = color.NRGBA{R: q[2], G: q[1], B: q[0], A: 0xff}
		}
	}

	xorStride := ((bpp*width + 31) / 32) * 4
	maskStride := ((width + 31) / 32) * 4
	maskOffset := headerSize + len(palette)*4 + xorStride*height

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		// DIB 是自下而上存储的
		rowStart := headerSize + len(palette)*4 + (height-1-y)*xorStride
		if rowStart+xorStride > len(raw) {
			break
		}
		row := raw[rowStart : rowStart+xorStride]
		for x := 0; x < width; x++ {
			var c color.NRGBA
			switch bpp {
			case 32:
				p := row[x*4:]
				c = color.NRGBA{R: p[2], G: p[1], B: p[0], A: p[3]}
			case 24:
				p := row[x*3:]
				c = color.NRGBA{R: p[2], G: p[1], B: p[0], A: 0xff}
			case 8:
				idx := int(row[x])
				if idx < len(palette) {
					c = palette[idx]
				}
			case 4:
				idx := int(row[x/2])
				if x%2 == 0 {
					idx >>= 4
				} else {
					idx &= 0x0f
				}
				if idx < len(palette) {
					c = palette[idx]
				}
			case 1:
				idx := (int(row[x/8]) >> uint(7-x%8)) & 0x01
				if idx < len(palette) {
					c = palette[idx]
				}
			}
			img.SetNRGBA(x, y, c)
		}
	}

	// 24 位及以下的图标用 AND 掩码表示透明区域
	if bpp <= 24 && maskOffset+maskStride*height <= len(raw) {
		for y := 0; y < height; y++ {
			rowStart := maskOffset + (height-1-y)*maskStride
			if rowStart+maskStride > len(raw) {
				break
			}
			row := raw[rowStart : rowStart+maskStride]
			for x := 0; x < width; x++ {
				if (int(row[x/8])>>uint(7-x%8))&0x01 == 1 {
					img.SetNRGBA(x, y, color.NRGBA{})
				}
			}
		}
	}
	return img, nil
}
