package app

import (
	"bytes"
	"errors"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

type HistVals struct {
	Title   string
	Buckets int
	Vals    map[int]int64
}

func NewHistVals(title string, buckets int) *HistVals {
	return &HistVals{
		Title:   title,
		Buckets: buckets,
		Vals:    map[int]int64{},
	}
}

func (h *HistVals) Len() int {
	return len(h.Vals)
}

func (h *HistVals) Inc(p int) {
	if _, ok := h.Vals[p]; !ok {
		h.Vals[p] = 1
	} else {
		h.Vals[p] += 1
	}
}

func (h *HistVals) XYer() plotter.XYer {
	xys := plotter.XYs{}
	type xy struct{ X, Y float64 }
	for x, y := range h.Vals {
		xys = append(xys, xy{X: float64(x), Y: float64(y)})
	}
	return xys
}

func (a *App) SendHist(camIndex int, hist *HistVals) error {
	if hist.Len() <= 0 {
		return errors.New("no data")
	}

	// Make a plot and set its title.
	p, err := plot.New()
	if err != nil {
		return err
	}
	p.Title.Text = hist.Title
	h, err := plotter.NewHistogram(hist.XYer(), 40)
	if err != nil {
		return err
	}
	p.Add(h)

	b := &bytes.Buffer{}
	w, err := p.WriterTo(8*vg.Inch, 8*vg.Inch, "jpg")
	if err != nil {
		return err
	}
	n, err := w.WriteTo(b)
	if err != nil {
		return err
	}
	if n <= 0 {
		return errors.New("zero bytes")
	}
	a.Cams[camIndex].Bot.SendImageBytesBuf(b)
	return nil
}
