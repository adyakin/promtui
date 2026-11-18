package graph

import (
	"bytes"
	"errors"
	"image/color"
	"image/png"
	"math"
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

// Exercise the actual raster, including flat-domain expansion and broken paths.
func TestFlatSeriesWithGap(t *testing.T) {
	start := time.Unix(0, 0)
	for _, missing := range []bool{true, false} {
		s := []model.SamplePair{{Timestamp: 0, Value: 42}, {Timestamp: 1000, Value: 42}}
		if !missing {
			s = append(s, model.SamplePair{Timestamp: 2000, Value: model.SampleValue(math.NaN())})
		}
		s = append(s, model.SamplePair{Timestamp: 3000, Value: 42}, model.SamplePair{Timestamp: 4000, Value: 42})
		p := Plot{start, start.Add(4 * time.Second), time.Second, []Line{{0, s}}}
		b, err := p.Render(t.Context(), 640, 320, 14)
		if err != nil {
			t.Fatal(err)
		}
		im, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		hits := [3]int{}
		for x := 0; x < 640; x++ {
			for y := 0; y < 320; y++ {
				c := color.RGBAModel.Convert(im.At(x, y)).(color.RGBA)
				if c.B > 200 && c.G > 160 && c.R < 155 {
					switch {
					case x < 250:
						hits[0]++
					case x > 450:
						hits[2]++
					case x > 300 && x < 400:
						hits[1]++
					}
				}
			}
		}
		if hits[0] == 0 || hits[1] != 0 || hits[2] == 0 {
			t.Fatalf("gap=%v line pixels left/middle/right: %v", missing, hits)
		}
	}
}
func TestNoFiniteData(t *testing.T) {
	p := Plot{Start: time.Now(), End: time.Now().Add(time.Hour), Lines: []Line{{Samples: []model.SamplePair{{Value: model.SampleValue(math.Inf(1))}}}}}
	if _, err := p.Render(t.Context(), 480, 200, 12); !errors.Is(err, ErrNoData) {
		t.Fatal(err)
	}
}
