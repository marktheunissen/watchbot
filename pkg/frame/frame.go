package frame

import (
	"fmt"
	"image"
	"strings"

	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("component", "frame")

type FrameDetectResult struct {
	Boxes     []*Box
	JPEGBytes []byte
}

func (f *FrameDetectResult) RejectSize(minWidth int, minHeight int, maxWidth int, maxHeight int) {
	for _, box := range f.Boxes {
		if minWidth != 0 && box.GetWidth() < minWidth {
			box.RejectReason = fmt.Sprintf("Rejected width: %d, %s", box.GetWidth(), box.LabelConfidence())
		} else if minHeight != 0 && box.GetHeight() < minHeight {
			box.RejectReason = fmt.Sprintf("Rejected height: %d, %s", box.GetWidth(), box.LabelConfidence())
		} else if maxWidth != 0 && box.GetWidth() > maxWidth {
			box.RejectReason = fmt.Sprintf("Rejected width: %d, %s", box.GetWidth(), box.LabelConfidence())
		} else if maxHeight != 0 && box.GetHeight() > maxHeight {
			box.RejectReason = fmt.Sprintf("Rejected height: %d, %s", box.GetHeight(), box.LabelConfidence())
		}
	}
}

func (f *FrameDetectResult) RejectOrientation(requirePortrait bool) {
	if !requirePortrait {
		return
	}
	for _, box := range f.Boxes {
		if !box.IsPortrait() {
			box.RejectReason = fmt.Sprintf("Rejected landscape: %d > %d, %s", box.GetWidth(), box.GetHeight(), box.LabelConfidence())
		}
	}
}

func (f *FrameDetectResult) RejectOutsideROI(roi *image.Rectangle) {
	if roi == nil {
		return
	}
	for _, box := range f.Boxes {
		if !box.Coords.In(*roi) {
			box.RejectReason = fmt.Sprintf("Rejected bounds: %v outside ROI: %v, %s", box.Coords, roi, box.LabelConfidence())
		}
	}
}

func (f *FrameDetectResult) RejectLowConfidence(minConfidence int) {
	if minConfidence == 0 {
		return
	}
	for _, box := range f.Boxes {
		if box.Confidence < minConfidence {
			box.RejectReason = fmt.Sprintf("Rejected confidence: %d below threshold: %d, %s", box.Confidence, minConfidence, box.LabelConfidence())
		}
	}
}

func (f *FrameDetectResult) ParseAlerts(alertLabels []string) {
	result := []*Box{}
	for _, box := range f.Boxes {
		found := false
		for _, l := range alertLabels {
			if box.Label == l {
				found = true
				break
			}
		}
		if found {
			log.Debugf("label: alerting: %s", box.Label)
			result = append(result, box)
		} else {
			log.Debugf("label: ignoring: %s", box.Label)
		}
	}
	f.Boxes = result
}

func (f *FrameDetectResult) RejectedBoxes() []*Box {
	ret := []*Box{}
	for _, box := range f.Boxes {
		if box.RejectReason != "" {
			ret = append(ret, box)
		}
	}
	return ret
}

func (f *FrameDetectResult) HitBoxes() []*Box {
	ret := []*Box{}
	for _, box := range f.Boxes {
		if box.RejectReason == "" {
			ret = append(ret, box)
		}
	}
	return ret
}

type Box struct {
	Label        string
	Confidence   int
	Coords       image.Rectangle
	JPEGBytes    []byte
	RejectReason string
}

func (b *Box) GetWidth() int {
	return b.Coords.Dx()
}

func (b *Box) GetHeight() int {
	return b.Coords.Dy()
}

func (b *Box) IsPortrait() bool {
	return b.GetWidth() < b.GetHeight()
}

func (b *Box) LabelPretty() string {
	return strings.Title(strings.ToLower(b.Label))
}

func (b *Box) LabelConfidence() string {
	return fmt.Sprintf("%s: %d%%", b.LabelPretty(), b.Confidence)
}
