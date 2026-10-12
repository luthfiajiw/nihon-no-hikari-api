package question

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
)

const maxStrokePoints = 256

// EvaluateStrokeWriting checks every stroke against its corresponding guide path.
// Coordinates and thresholds are in the KanjiVG 109 x 109 view box.
func EvaluateStrokeWriting(guide WritingGuide, input json.RawMessage) (bool, float64, error) {
	if len(guide.Strokes) == 0 || guide.ViewBox != 109 {
		return false, 0, errors.New("panduan stroke tidak valid")
	}
	if len(input) == 0 {
		return false, 0, nil
	}
	var strokes [][]StrokePoint
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&strokes); err != nil || strokes == nil {
		return false, 0, errors.New("stroke_input harus berupa array stroke")
	}
	if len(strokes) != len(guide.Strokes) {
		return false, 0, nil
	}
	var coverageTotal float64
	allValid := true
	for index, stroke := range strokes {
		if len(stroke) < 3 || len(stroke) > maxStrokePoints {
			return false, 0, errors.New("setiap stroke harus berisi 3 hingga 256 titik")
		}
		for _, point := range stroke {
			if math.IsNaN(point.X) || math.IsInf(point.X, 0) || math.IsNaN(point.Y) || math.IsInf(point.Y, 0) ||
				point.X < 0 || point.X > guide.ViewBox || point.Y < 0 || point.Y > guide.ViewBox {
				return false, 0, errors.New("koordinat stroke di luar panduan")
			}
		}
		reference := guide.Strokes[index].Points
		if len(reference) != 32 {
			return false, 0, errors.New("panduan stroke tidak valid")
		}
		samples, inputLength := resampleStroke(stroke, 32)
		_, referenceLength := resampleStroke(reference, 32)
		if referenceLength <= 0 || inputLength <= 0 {
			allValid = false
			continue
		}
		matches := 0
		for sample := range samples {
			if strokeDistance(samples[sample], reference[sample]) <= 10 {
				matches++
			}
		}
		coverage := float64(matches) / 32
		coverageTotal += coverage
		if coverage < 0.85 || inputLength/referenceLength < 0.7 || inputLength/referenceLength > 1.3 ||
			strokeDistance(stroke[0], reference[0]) > 8 ||
			strokeDistance(stroke[len(stroke)-1], reference[len(reference)-1]) > 8 {
			allValid = false
		}
	}
	return allValid, math.Round(coverageTotal/float64(len(strokes))*10000) / 100, nil
}

func strokeDistance(a, b StrokePoint) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}

func resampleStroke(points []StrokePoint, count int) ([]StrokePoint, float64) {
	lengths := make([]float64, len(points))
	for index := 1; index < len(points); index++ {
		lengths[index] = lengths[index-1] + strokeDistance(points[index-1], points[index])
	}
	total := lengths[len(lengths)-1]
	output := make([]StrokePoint, count)
	if total == 0 {
		for index := range output {
			output[index] = points[0]
		}
		return output, 0
	}
	segment := 1
	for index := range output {
		target := total * float64(index) / float64(count-1)
		for segment < len(points)-1 && lengths[segment] < target {
			segment++
		}
		span := lengths[segment] - lengths[segment-1]
		fraction := 0.0
		if span > 0 {
			fraction = (target - lengths[segment-1]) / span
		}
		output[index] = StrokePoint{
			X: points[segment-1].X + fraction*(points[segment].X-points[segment-1].X),
			Y: points[segment-1].Y + fraction*(points[segment].Y-points[segment-1].Y),
		}
	}
	return output, total
}
