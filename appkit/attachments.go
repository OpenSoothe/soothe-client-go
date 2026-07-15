package appkit

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
	"image/png"
	"strings"

	"golang.org/x/image/draw"
)

// CompactImageOptions controls CompactImageAttachment. Zero fields use defaults.
type CompactImageOptions struct {
	// MaxDim is the maximum width or height in pixels. Default 768.
	MaxDim int
	// JPEGQuality is the JPEG encode quality (1–100). Default 85.
	JPEGQuality int
}

func compactDefaults(opts *CompactImageOptions) (maxDim, quality int) {
	maxDim, quality = 768, 85
	if opts == nil {
		return maxDim, quality
	}
	if opts.MaxDim > 0 {
		maxDim = opts.MaxDim
	}
	if opts.JPEGQuality > 0 {
		quality = opts.JPEGQuality
	}
	return maxDim, quality
}

// CompactImageAttachment downscales image/* payloads when either dimension
// exceeds MaxDim. Non-images and decode failures pass through unchanged.
// PNG stays PNG; other image types re-encode as JPEG.
func CompactImageAttachment(mimeType, dataB64 string, opts *CompactImageOptions) (outMime, outB64 string) {
	if dataB64 == "" || !strings.HasPrefix(mimeType, "image/") {
		return mimeType, dataB64
	}
	raw, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil || len(raw) == 0 {
		return mimeType, dataB64
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return mimeType, dataB64
	}
	maxDim, quality := compactDefaults(opts)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxDim && h <= maxDim {
		return mimeType, dataB64
	}
	nw, nh := w, h
	if w >= h {
		if w > maxDim {
			nw = maxDim
			nh = h * maxDim / w
		}
	} else if h > maxDim {
		nh = maxDim
		nw = w * maxDim / h
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)

	var buf bytes.Buffer
	outMime = mimeType
	switch mimeType {
	case "image/png":
		if err := png.Encode(&buf, dst); err != nil {
			return mimeType, dataB64
		}
	default:
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
			return mimeType, dataB64
		}
		outMime = "image/jpeg"
	}
	return outMime, base64.StdEncoding.EncodeToString(buf.Bytes())
}

// CompactAttachments applies CompactImageAttachment to each attachment map
// that carries mime_type + data (base64) keys.
func CompactAttachments(atts []map[string]interface{}, opts *CompactImageOptions) []map[string]interface{} {
	if len(atts) == 0 {
		return atts
	}
	out := make([]map[string]interface{}, len(atts))
	for i, att := range atts {
		cp := make(map[string]interface{}, len(att))
		for k, v := range att {
			cp[k] = v
		}
		mime, _ := cp["mime_type"].(string)
		data, _ := cp["data"].(string)
		if mime != "" && data != "" {
			outMime, outData := CompactImageAttachment(mime, data, opts)
			cp["mime_type"] = outMime
			cp["data"] = outData
		}
		out[i] = cp
	}
	return out
}
