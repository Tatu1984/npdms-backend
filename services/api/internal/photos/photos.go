// Package photos checks and prepares photographs of people before they are
// stored: the type is decided by the file's own bytes, location metadata is
// removed, and a small JPEG thumbnail is drawn on the server.
//
// Location metadata is removed without re-encoding the picture, so the image
// data a family handed over is kept exactly:
//
//   - EXIF (JPEG APP1, PNG eXIf, WebP EXIF): the GPS directory is emptied in
//     place — its entries and the values they point at are overwritten with
//     zeros. Other EXIF fields, including orientation, are kept.
//   - XMP (JPEG APP1, PNG iTXt, WebP "XMP "), which can repeat the GPS position,
//     is dropped whole, as are PNG "Raw profile type exif/xmp" text chunks.
package photos

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// MaxBytes is the largest photograph accepted.
const MaxBytes = 8 << 20

// maxPixels guards against decompression bombs: a small file that declares an
// enormous canvas.
const maxPixels = 60_000_000

var (
	ErrUnsupportedType = errors.New("only JPEG, PNG and WebP photographs are accepted")
	ErrUnreadable      = errors.New("the file could not be read as a photograph")
	ErrTooManyPixels   = errors.New("the photograph's dimensions are too large")
)

// Prepared is a checked photograph ready to store.
type Prepared struct {
	ContentType string
	Width       int
	Height      int
	// Data is the file with location metadata removed.
	Data            []byte
	LocationRemoved bool
	Note            string
	// Orientation is the EXIF orientation (1–8), 1 when absent.
	Orientation int
}

// Sniff decides the type from magic bytes alone.
func Sniff(b []byte) (string, error) {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "image/jpeg", nil
	case len(b) >= 8 && bytes.Equal(b[:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return "image/png", nil
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp", nil
	}
	return "", ErrUnsupportedType
}

// Prepare checks the bytes, removes location metadata and reads dimensions.
func Prepare(raw []byte) (*Prepared, error) {
	ct, err := Sniff(raw)
	if err != nil {
		return nil, err
	}
	var cfg image.Config
	switch ct {
	case "image/jpeg":
		cfg, err = jpeg.DecodeConfig(bytes.NewReader(raw))
	case "image/png":
		cfg, err = png.DecodeConfig(bytes.NewReader(raw))
	case "image/webp":
		cfg, err = webp.DecodeConfig(bytes.NewReader(raw))
	}
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, ErrUnreadable
	}
	if cfg.Width*cfg.Height > maxPixels {
		return nil, ErrTooManyPixels
	}

	var s scrub
	var data []byte
	switch ct {
	case "image/jpeg":
		data, err = s.jpeg(raw)
	case "image/png":
		data, err = s.png(raw)
	case "image/webp":
		data, err = s.webp(raw)
	}
	if err != nil {
		return nil, ErrUnreadable
	}
	// The cleaned file must still decode: a scrub that damaged it is refused
	// rather than stored.
	if _, err := decode(ct, data); err != nil {
		return nil, ErrUnreadable
	}
	return &Prepared{
		ContentType: ct, Width: cfg.Width, Height: cfg.Height, Data: data,
		LocationRemoved: s.gps || s.xmp, Note: s.note(), Orientation: s.orientation(),
	}, nil
}

func decode(ct string, data []byte) (image.Image, error) {
	switch ct {
	case "image/jpeg":
		return jpeg.Decode(bytes.NewReader(data))
	case "image/png":
		return png.Decode(bytes.NewReader(data))
	case "image/webp":
		return webp.Decode(bytes.NewReader(data))
	}
	return nil, ErrUnsupportedType
}

// Thumbnail draws a JPEG no larger than max pixels on its longer side, upright
// according to the EXIF orientation.
func Thumbnail(data []byte, max int) ([]byte, error) {
	ct, err := Sniff(data)
	if err != nil {
		return nil, err
	}
	src, err := decode(ct, data)
	if err != nil {
		return nil, ErrUnreadable
	}
	var s scrub
	// Only to read orientation; the bytes are not changed here.
	cp := append([]byte(nil), data...)
	switch ct {
	case "image/jpeg":
		_, _ = s.jpeg(cp)
	case "image/png":
		_, _ = s.png(cp)
	case "image/webp":
		_, _ = s.webp(cp)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := float64(max) / float64(w)
	if h > w {
		scale = float64(max) / float64(h)
	}
	if scale > 1 {
		scale = 1
	}
	tw, th := int(float64(w)*scale+0.5), int(float64(h)*scale+0.5)
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	// A transparent PNG is laid on white rather than black.
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	out := orient(dst, s.orientation())
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// orient applies an EXIF orientation to an image.
func orient(src *image.RGBA, o int) image.Image {
	if o <= 1 || o > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.SetRGBA(dx, dy, src.RGBAAt(x, y))
		}
	}
	return dst
}

/* ------------------------------------------------------------------ scrub */

type scrub struct {
	gps    bool
	xmp    bool
	orient int
}

func (s *scrub) orientation() int {
	if s.orient < 1 || s.orient > 8 {
		return 1
	}
	return s.orient
}

func (s *scrub) note() string {
	parts := []string{}
	if s.gps {
		parts = append(parts, "GPS location removed from EXIF")
	}
	if s.xmp {
		parts = append(parts, "XMP metadata removed")
	}
	if len(parts) == 0 {
		return "No location metadata found"
	}
	return strings.Join(parts, "; ")
}

var (
	exifHeader   = []byte("Exif\x00\x00")
	xmpHeader    = []byte("http://ns.adobe.com/xap/1.0/\x00")
	xmpExtHeader = []byte("http://ns.adobe.com/xmp/extension/\x00")
)

func (s *scrub) jpeg(b []byte) ([]byte, error) {
	if len(b) < 4 {
		return nil, ErrUnreadable
	}
	out := make([]byte, 0, len(b))
	out = append(out, b[:2]...)
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return nil, ErrUnreadable
		}
		marker := b[i+1]
		if marker == 0xFF { // fill byte
			i++
			continue
		}
		// Start of scan (or end of image): everything after is image data.
		if marker == 0xDA || marker == 0xD9 {
			out = append(out, b[i:]...)
			return out, nil
		}
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			out = append(out, b[i:i+2]...)
			i += 2
			continue
		}
		length := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		end := i + 2 + length
		if length < 2 || end > len(b) {
			return nil, ErrUnreadable
		}
		seg := b[i:end]
		payload := b[i+4 : end]
		if marker == 0xE1 {
			switch {
			case bytes.HasPrefix(payload, exifHeader):
				s.tiff(payload[len(exifHeader):])
			case bytes.HasPrefix(payload, xmpHeader), bytes.HasPrefix(payload, xmpExtHeader):
				s.xmp = true
				i = end
				continue
			}
		}
		out = append(out, seg...)
		i = end
	}
	return nil, ErrUnreadable
}

func (s *scrub) png(b []byte) ([]byte, error) {
	if len(b) < 8 {
		return nil, ErrUnreadable
	}
	out := make([]byte, 0, len(b))
	out = append(out, b[:8]...)
	i := 8
	for i+12 <= len(b) {
		length := int(binary.BigEndian.Uint32(b[i : i+4]))
		end := i + 12 + length
		if length < 0 || end > len(b) {
			return nil, ErrUnreadable
		}
		typ := string(b[i+4 : i+8])
		data := b[i+8 : i+8+length]
		switch typ {
		case "eXIf":
			if s.tiff(data) {
				binary.BigEndian.PutUint32(b[i+8+length:end], crc32.ChecksumIEEE(b[i+4:i+8+length]))
			}
		case "iTXt", "tEXt", "zTXt":
			keyword := data
			if k := bytes.IndexByte(data, 0); k >= 0 {
				keyword = data[:k]
			}
			kw := string(keyword)
			if kw == "XML:com.adobe.xmp" || strings.HasPrefix(kw, "Raw profile type exif") || strings.HasPrefix(kw, "Raw profile type xmp") {
				s.xmp = true
				i = end
				continue
			}
		}
		out = append(out, b[i:end]...)
		i = end
		if typ == "IEND" {
			return out, nil
		}
	}
	return nil, ErrUnreadable
}

func (s *scrub) webp(b []byte) ([]byte, error) {
	if len(b) < 12 {
		return nil, ErrUnreadable
	}
	out := make([]byte, 0, len(b))
	out = append(out, b[:12]...)
	vp8x := -1
	i := 12
	for i+8 <= len(b) {
		fourcc := string(b[i : i+4])
		size := int(binary.LittleEndian.Uint32(b[i+4 : i+8]))
		padded := size + size%2
		end := i + 8 + padded
		if size < 0 || i+8+size > len(b) {
			return nil, ErrUnreadable
		}
		if end > len(b) {
			end = len(b)
		}
		data := b[i+8 : i+8+size]
		switch fourcc {
		case "VP8X":
			vp8x = len(out)
		case "EXIF":
			t := data
			if bytes.HasPrefix(t, exifHeader) {
				t = t[len(exifHeader):]
			}
			s.tiff(t)
		case "XMP ":
			s.xmp = true
			i = end
			continue
		}
		out = append(out, b[i:end]...)
		i = end
	}
	if s.xmp && vp8x >= 0 && vp8x+8 < len(out) {
		out[vp8x+8] &^= 0x04 // XMP flag
	}
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out, nil
}

// tiff empties the GPS directory of a TIFF/EXIF block in place and reads the
// orientation. It reports whether anything was changed. Malformed blocks are
// left as they are; decoding the image does not depend on them.
func (s *scrub) tiff(t []byte) bool {
	if len(t) < 8 {
		return false
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return false
	}
	changed := false
	offset := int(bo.Uint32(t[4:8]))
	for ifd := 0; ifd < 4 && offset > 0 && offset+2 <= len(t); ifd++ {
		n := int(bo.Uint16(t[offset : offset+2]))
		base := offset + 2
		if base+n*12+4 > len(t) {
			return changed
		}
		for e := 0; e < n; e++ {
			entry := t[base+e*12 : base+e*12+12]
			tag := bo.Uint16(entry[0:2])
			switch tag {
			case 0x0112: // orientation, SHORT
				if ifd == 0 {
					s.orient = int(bo.Uint16(entry[8:10]))
				}
			case 0x8825: // GPS IFD pointer
				if emptyIFD(t, bo, int(bo.Uint32(entry[8:12]))) {
					s.gps, changed = true, true
				}
			}
		}
		offset = int(bo.Uint32(t[base+n*12 : base+n*12+4]))
	}
	return changed
}

// emptyIFD zeroes every entry of the directory at off and the out-of-line
// values they point to, then sets its entry count to zero.
func emptyIFD(t []byte, bo binary.ByteOrder, off int) bool {
	if off <= 0 || off+2 > len(t) {
		return false
	}
	n := int(bo.Uint16(t[off : off+2]))
	base := off + 2
	if n == 0 || base+n*12 > len(t) {
		return false
	}
	for e := 0; e < n; e++ {
		entry := t[base+e*12 : base+e*12+12]
		size := typeSize(bo.Uint16(entry[2:4])) * int(bo.Uint32(entry[4:8]))
		if size > 4 {
			vo := int(bo.Uint32(entry[8:12]))
			if vo > 0 && size > 0 && vo+size <= len(t) {
				clear(t[vo : vo+size])
			}
		}
		clear(entry)
	}
	bo.PutUint16(t[off:off+2], 0)
	return true
}

func typeSize(typ uint16) int {
	switch typ {
	case 1, 2, 6, 7: // BYTE, ASCII, SBYTE, UNDEFINED
		return 1
	case 3, 8: // SHORT, SSHORT
		return 2
	case 4, 9, 11: // LONG, SLONG, FLOAT
		return 4
	case 5, 10, 12: // RATIONAL, SRATIONAL, DOUBLE
		return 8
	}
	return 0
}

// Describe renders a byte count for messages.
func Describe(n int64) string {
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}
