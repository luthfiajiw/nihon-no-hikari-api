// Package kanastroke converts KanjiVG SVGs into the writing evaluator's format.
package kanastroke

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxSVGBytes = 256 * 1024

var ErrTooLarge = errors.New("ukuran SVG maksimal 256 KiB")

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Stroke struct {
	Path   string  `json:"path"`
	Points []Point `json:"points"`
}

type Data struct {
	ViewBox float64  `json:"view_box"`
	Strokes []Stroke `json:"strokes"`
}

func ValidCharacter(script, character string) bool {
	if !utf8.ValidString(character) || utf8.RuneCountInString(character) != 1 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(character)
	return unicode.IsLetter(r) && ((script == "hiragana" && unicode.Is(unicode.Hiragana, r)) ||
		(script == "katakana" && unicode.Is(unicode.Katakana, r)))
}

func attr(element xml.StartElement, name string) string {
	for _, attribute := range element.Attr {
		if attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}

// Convert reads only stroke geometry. XML directives are never resolved, and no
// uploaded markup, styles, scripts, or external resources reach the renderer.
func Convert(svg, script, character string) (Data, error) {
	data := Data{ViewBox: 109, Strokes: []Stroke{}}
	if len(svg) > MaxSVGBytes {
		return data, ErrTooLarge
	}
	if !ValidCharacter(script, character) {
		return data, errors.New("karakter harus berupa satu kana sesuai script")
	}
	r, _ := utf8.DecodeRuneInString(character)
	code := fmt.Sprintf("%05x", r)
	groupID := "kvg:StrokePaths_" + code
	decoder := xml.NewDecoder(strings.NewReader(svg))
	depth, strokeDepth, groups := 0, 0, 0
	rootSeen, rootClosed := false, false
	transforms := []bool{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return data, errors.New("SVG bukan XML yang valid")
		}
		switch element := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				if rootSeen || element.Name.Local != "svg" || element.Name.Space != "http://www.w3.org/2000/svg" {
					return data, errors.New("dokumen harus berisi satu elemen SVG")
				}
				rootSeen = true
				box := strings.Fields(strings.ReplaceAll(attr(element, "viewBox"), ",", " "))
				if len(box) != 4 {
					return data, errors.New("viewBox SVG harus 0 0 109 109")
				}
				for i, expected := range []float64{0, 0, 109, 109} {
					value, err := strconv.ParseFloat(box[i], 64)
					if err != nil || value != expected {
						return data, errors.New("viewBox SVG harus 0 0 109 109")
					}
				}
			}
			depth++
			transformed := strings.TrimSpace(attr(element, "transform")) != "" ||
				strings.Contains(strings.ToLower(attr(element, "style")), "transform")
			if len(transforms) > 0 {
				transformed = transformed || transforms[len(transforms)-1]
			}
			transforms = append(transforms, transformed)
			if depth > 64 {
				return data, errors.New("struktur SVG terlalu dalam")
			}
			if strings.HasPrefix(attr(element, "id"), "kvg:StrokePaths_") {
				if element.Name.Local != "g" || attr(element, "id") != groupID || groups != 0 {
					return data, errors.New("identitas StrokePaths SVG tidak cocok dengan karakter")
				}
				groups++
				strokeDepth = depth
			}
			if strokeDepth != 0 {
				if transformed {
					return data, errors.New("transformasi pada jalur goresan belum didukung")
				}
				if element.Name.Space != "http://www.w3.org/2000/svg" ||
					(element.Name.Local != "g" && element.Name.Local != "path") {
					return data, errors.New("StrokePaths hanya boleh berisi grup dan path")
				}
				if element.Name.Local == "path" {
					expectedID := fmt.Sprintf("kvg:%s-s%d", code, len(data.Strokes)+1)
					if attr(element, "id") != expectedID {
						return data, errors.New("nomor goresan SVG harus berurutan mulai dari 1")
					}
					path := strings.TrimSpace(attr(element, "d"))
					points, err := samplePath(path)
					if err != nil {
						return data, fmt.Errorf("goresan %d: %w", len(data.Strokes)+1, err)
					}
					data.Strokes = append(data.Strokes, Stroke{Path: path, Points: points})
				}
			}
		case xml.EndElement:
			if depth == strokeDepth {
				strokeDepth = 0
			}
			depth--
			transforms = transforms[:len(transforms)-1]
			if depth == 0 {
				rootClosed = true
			}
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(element)) != "" {
				return data, errors.New("teks di luar elemen SVG tidak valid")
			}
		}
	}
	if !rootSeen || !rootClosed || groups != 1 {
		return data, errors.New("grup StrokePaths KanjiVG tidak ditemukan")
	}
	return data, Validate(data)
}

// Validate also protects grading from malformed stored guides and snapshots.
func Validate(data Data) error {
	if data.ViewBox != 109 || len(data.Strokes) == 0 {
		return errors.New("panduan stroke tidak valid")
	}
	for _, stroke := range data.Strokes {
		if strings.TrimSpace(stroke.Path) == "" || len(stroke.Points) != 32 {
			return errors.New("setiap goresan harus memiliki path dan 32 titik acuan")
		}
		length := 0.0
		for i, point := range stroke.Points {
			if !finite(point.X) || !finite(point.Y) || point.X < 0 || point.X > 109 || point.Y < 0 || point.Y > 109 {
				return errors.New("koordinat goresan di luar bidang 0–109")
			}
			if i > 0 {
				length += distance(stroke.Points[i-1], point)
			}
		}
		if length <= 0 {
			return errors.New("panjang goresan harus lebih dari nol")
		}
	}
	return nil
}

func finite(value float64) bool   { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func distance(a, b Point) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }

var pathToken = regexp.MustCompile(`[MmCcSs]|[-+]?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?`)

func samplePath(path string) ([]Point, error) {
	// Verify every byte is tokenized; unsupported commands must not be skipped.
	indices := pathToken.FindAllStringIndex(path, -1)
	tokens := make([]string, 0, len(indices))
	end := 0
	for _, span := range indices {
		gap := path[end:span[0]]
		if strings.Trim(gap, " \t\r\n,") != "" {
			return nil, errors.New("perintah path belum didukung; gunakan M/m, C/c, S/s")
		}
		if strings.Count(gap, ",") > 1 || (strings.Contains(gap, ",") &&
			(len(tokens) == 0 || strings.ContainsAny(path[span[0]:span[1]], "MmCcSs") || strings.ContainsAny(tokens[len(tokens)-1], "MmCcSs"))) {
			return nil, errors.New("pemisah parameter path tidak valid")
		}
		tokens = append(tokens, path[span[0]:span[1]])
		end = span[1]
	}
	if strings.TrimSpace(path[end:]) != "" || len(tokens) < 3 || (tokens[0] != "M" && tokens[0] != "m") {
		return nil, errors.New("path harus diawali satu M/m dan berisi kurva cubic")
	}
	number := func(token string) (float64, error) {
		value, err := strconv.ParseFloat(token, 64)
		if err != nil || !finite(value) || math.Abs(value) > 1000000 {
			return 0, errors.New("angka path tidak valid")
		}
		return value, nil
	}
	x, err := number(tokens[1])
	if err != nil {
		return nil, err
	}
	y, err := number(tokens[2])
	if err != nil {
		return nil, err
	}
	current := Point{x, y}
	dense := []Point{current}
	previousControl := current
	command := ""
	for index := 3; index < len(tokens); {
		if len(tokens[index]) == 1 && strings.ContainsAny(tokens[index], "MmCcSs") {
			command = tokens[index]
			index++
			if command == "M" || command == "m" {
				return nil, errors.New("satu goresan hanya boleh memiliki satu moveto")
			}
		}
		count := 6
		if command == "S" || command == "s" {
			count = 4
		}
		if command == "" || index+count > len(tokens) {
			return nil, errors.New("parameter kurva cubic tidak lengkap")
		}
		values := make([]float64, count)
		for i := range values {
			values[i], err = number(tokens[index+i])
			if err != nil {
				return nil, err
			}
		}
		relative := command == "c" || command == "s"
		point := func(i int) Point {
			p := Point{values[i], values[i+1]}
			if relative {
				p.X += current.X
				p.Y += current.Y
			}
			return p
		}
		control1 := Point{2*current.X - previousControl.X, 2*current.Y - previousControl.Y}
		control2, next := point(0), point(2)
		if count == 6 {
			control1, control2, next = point(0), point(2), point(4)
		}
		for step := 1; step <= 30; step++ {
			t := float64(step) / 30
			u := 1 - t
			dense = append(dense, Point{
				X: u*u*u*current.X + 3*u*u*t*control1.X + 3*u*t*t*control2.X + t*t*t*next.X,
				Y: u*u*u*current.Y + 3*u*u*t*control1.Y + 3*u*t*t*control2.Y + t*t*t*next.Y,
			})
		}
		current, previousControl = next, control2
		index += count
	}
	lengths := make([]float64, len(dense))
	for i := 1; i < len(dense); i++ {
		lengths[i] = lengths[i-1] + distance(dense[i-1], dense[i])
	}
	total := lengths[len(lengths)-1]
	if !finite(total) || total <= 0 {
		return nil, errors.New("panjang goresan harus lebih dari nol")
	}
	points := make([]Point, 32)
	segment := 1
	for sample := range points {
		target := total * float64(sample) / 31
		for segment < len(lengths)-1 && (lengths[segment] < target || lengths[segment] == lengths[segment-1]) {
			segment++
		}
		span := lengths[segment] - lengths[segment-1]
		fraction := 0.0
		if span > 0 {
			fraction = (target - lengths[segment-1]) / span
		}
		points[sample] = Point{
			X: math.Round((dense[segment-1].X+fraction*(dense[segment].X-dense[segment-1].X))*1000) / 1000,
			Y: math.Round((dense[segment-1].Y+fraction*(dense[segment].Y-dense[segment-1].Y))*1000) / 1000,
		}
	}
	return points, nil
}
