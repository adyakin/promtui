// Package graph renders timestamped numerical series without knowing about
// Bubble Tea, terminal cells or the image transport.
package graph

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"math"
	"time"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/prometheus/common/model"
	"golang.org/x/image/font/gofont/goregular"
)

type Line struct {
	Index   int
	Samples []model.SamplePair
}
type Plot struct {
	Start, End time.Time
	Step       time.Duration
	Lines      []Line
}

var ErrNoData = errors.New("no finite samples in the visible series")
var palette = []string{"#67d9ff", "#ffbf69", "#85df99", "#f39acc", "#aeabff", "#f78e79", "#f1dc75", "#76d5c2"}
var typeface = func() *truetype.Font {
	f, err := truetype.Parse(goregular.TTF)
	if err != nil {
		panic(err)
	}
	return f
}()

func Color(i int) string { return palette[i%len(palette)] }

// Bounds are the plot edges in image pixels, excluding the axis labels.
type Bounds struct {
	Left, Top, Right, Bottom float64
}

func (p Plot) Render(ctx context.Context, w, h int, fontSize float64) ([]byte, Bounds, error) {
	if w < 160 || h < 80 || w > 3200 || h > 2000 || !p.End.After(p.Start) {
		return nil, Bounds{}, fmt.Errorf("graph viewport too small or invalid")
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, s := range p.Lines {
		for i, v := range s.Samples {
			if i%1024 == 0 && ctx.Err() != nil {
				return nil, Bounds{}, ctx.Err()
			}
			y := float64(v.Value)
			if !math.IsNaN(y) && !math.IsInf(y, 0) {
				lo = min(lo, y)
				hi = max(hi, y)
			}
		}
	}
	if math.IsInf(lo, 1) {
		return nil, Bounds{}, ErrNoData
	}
	// Normalize before subtracting: even [-MaxFloat64, MaxFloat64] fits.
	scale := max(math.Abs(lo), math.Abs(hi))
	if scale == 0 {
		scale = 1
	}
	lo, hi = lo/scale, hi/scale
	if lo == hi {
		lo -= 0.05
		hi += 0.05
	}
	d := gg.NewContext(w, h)
	fontSize = max(10, min(fontSize, float64(h)/6))
	face := truetype.NewFace(typeface, &truetype.Options{Size: fontSize})
	defer face.Close()
	d.SetFontFace(face)
	d.SetHexColor("#19212b")
	d.Clear()
	label := func(frac float64) string {
		n := (lo + (hi-lo)*frac) * scale
		if math.IsInf(n, 0) {
			return ""
		}
		return fmt.Sprintf("%.4g", n)
	}
	left := 0.
	for i := 0; i <= 4; i++ {
		tw, _ := d.MeasureString(label(float64(i) / 4))
		left = max(left, tw)
	}
	left += fontSize
	right := float64(w) - fontSize
	top, bottom := fontSize, float64(h)-2*fontSize
	ny := max(1, min(4, int((bottom-top)/(2*fontSize))))
	for i := 0; i <= ny; i++ {
		f := float64(i) / float64(ny)
		y := bottom - f*(bottom-top)
		d.SetHexColor("#394958")
		d.SetLineWidth(1)
		d.DrawLine(left, y, right, y)
		d.Stroke()
		d.SetHexColor("#d7e0e8")
		d.DrawStringAnchored(label(f), left-fontSize/2, y, 1, 0.5)
	}
	span := p.End.Sub(p.Start)
	format := "15:04"
	if span >= 24*time.Hour {
		format = "Jan 02 15:04"
	} else if span < 2*time.Minute {
		format = "15:04:05"
	}
	tw, _ := d.MeasureString(p.Start.UTC().Format(format))
	nx := max(1, min(6, int((right-left)/(tw+fontSize*2))))
	for i := 0; i <= nx; i++ {
		f := float64(i) / float64(nx)
		x := left + f*(right-left)
		d.SetHexColor("#344350")
		d.DrawLine(x, top, x, bottom)
		d.Stroke()
		d.SetHexColor("#d7e0e8")
		d.DrawStringAnchored(p.Start.Add(time.Duration(f*float64(span))).UTC().Format(format), max(tw/2, min(x, float64(w)-tw/2-2)), float64(h)-fontSize/2, .5, 0)
	}
	for _, s := range p.Lines {
		d.SetHexColor(Color(s.Index))
		d.SetLineWidth(max(1.5, fontSize/7))
		var last time.Time
		var lx, ly float64
		count := 0
		flush := func() {
			if count > 1 {
				d.Stroke()
			} else if count == 1 {
				d.ClearPath()
				d.DrawCircle(lx, ly, max(2, fontSize/6))
				d.Fill()
			}
			count = 0
		}
		for i, v := range s.Samples {
			if i%1024 == 0 && ctx.Err() != nil {
				return nil, Bounds{}, ctx.Err()
			}
			t, y := v.Timestamp.Time(), float64(v.Value)
			if math.IsNaN(y) || math.IsInf(y, 0) {
				flush()
				continue
			}
			if count > 0 && t.Sub(last) > p.Step+p.Step/2 {
				flush()
			}
			lx = left + float64(t.Sub(p.Start))/float64(span)*(right-left)
			ly = bottom - (y/scale-lo)/(hi-lo)*(bottom-top)
			if count == 0 {
				d.MoveTo(lx, ly)
			} else {
				d.LineTo(lx, ly)
			}
			count++
			last = t
		}
		flush()
	}
	if err := ctx.Err(); err != nil {
		return nil, Bounds{}, err
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, d.Image()); err != nil {
		return nil, Bounds{}, err
	}
	return buf.Bytes(), Bounds{left, top, right, bottom}, nil
}
