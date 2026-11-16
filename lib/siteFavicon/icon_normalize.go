package siteFavicon

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"strings"

	xdraw "golang.org/x/image/draw"

	// 注册 gif / jpeg / webp / bmp 解码器
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
)

const (
	// StandardIconSize 是所有远程图标统一保存的边长（像素）。
	// 站点图标常见 16/32/48/180/192 等尺寸，统一到同一画布后，
	// 面板上的图标大小与清晰度才一致。
	StandardIconSize = 192
	// 候选图标预览缩略图边长
	previewSize = 64
)

// decodeIconImage 解码图标内容，支持 ICO 与常见位图格式
func decodeIconImage(data []byte) (image.Image, error) {
	if isICO(data) {
		return decodeICO(data)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// NormalizeIcon 把图标统一绘制到正方形画布并编码为 PNG。
// SVG 是矢量图，任意尺寸都清晰，直接原样返回。
func NormalizeIcon(data []byte) (out []byte, ext string, width, height int, err error) {
	if looksLikeSVG(data) {
		return data, ".svg", 0, 0, nil
	}
	img, err := decodeIconImage(data)
	if err != nil {
		return nil, "", 0, 0, err
	}
	src := img.Bounds()
	if src.Dx() <= 0 || src.Dy() <= 0 {
		return nil, "", 0, 0, errors.New("图标尺寸无效")
	}

	canvas := image.NewNRGBA(image.Rect(0, 0, StandardIconSize, StandardIconSize))
	xdraw.CatmullRom.Scale(canvas, fitRect(src.Dx(), src.Dy(), StandardIconSize), img, src, xdraw.Over, nil)

	var buf bytes.Buffer
	if err := png.Encode(&buf, canvas); err != nil {
		return nil, "", 0, 0, err
	}
	return buf.Bytes(), ".png", src.Dx(), src.Dy(), nil
}

// fitRect 计算等比缩放后居中的目标矩形
func fitRect(srcW, srcH, box int) image.Rectangle {
	if srcW <= 0 || srcH <= 0 {
		return image.Rect(0, 0, box, box)
	}
	w, h := box, box
	if srcW >= srcH {
		h = box * srcH / srcW
		if h < 1 {
			h = 1
		}
	} else {
		w = box * srcW / srcH
		if w < 1 {
			w = 1
		}
	}
	x := (box - w) / 2
	y := (box - h) / 2
	return image.Rect(x, y, x+w, y+h)
}

// MakePreview 生成 base64 的 PNG 缩略图，用于让用户挑选图标
func MakePreview(data []byte) string {
	if looksLikeSVG(data) {
		// 矢量图直接交给浏览器渲染，避免服务端栅格化依赖
		return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(data)
	}
	img, err := decodeIconImage(data)
	if err != nil {
		return ""
	}
	src := img.Bounds()
	if src.Dx() <= 0 || src.Dy() <= 0 {
		return ""
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, previewSize, previewSize))
	xdraw.CatmullRom.Scale(canvas, fitRect(src.Dx(), src.Dy(), previewSize), img, src, xdraw.Over, nil)

	var buf bytes.Buffer
	if err := png.Encode(&buf, canvas); err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// IconDimensions 读取图标原始尺寸（用于展示与排序）
func IconDimensions(data []byte) (int, int) {
	if looksLikeSVG(data) {
		return 0, 0
	}
	if isICO(data) {
		img, err := decodeICO(data)
		if err != nil {
			return 0, 0
		}
		b := img.Bounds()
		return b.Dx(), b.Dy()
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// looksLikeSVG 判断内容是否是 SVG（Go 的嗅探器不识别该格式）
func looksLikeSVG(data []byte) bool {
	head := data
	if len(head) > 2048 {
		head = head[:2048]
	}
	lower := bytes.ToLower(head)
	if !bytes.Contains(lower, []byte("<svg")) {
		return false
	}
	return !bytes.HasPrefix(lower, []byte("\x89png"))
}

// hashIcon 用于剔除内容完全相同的候选图标
func hashIcon(data []byte) string {
	sum := md5.Sum(data)
	return string(sum[:])
}

var _ = strings.TrimSpace
