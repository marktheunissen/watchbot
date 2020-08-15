package frame

import (
	"strings"
	"testing"

	"github.com/marktheunissen/watchbot/pkg/utils"
)

func TestRejectSize(t *testing.T) {
	r := utils.GetRect(0, 0, 300, 300)
	f := &FrameDetectResult{
		Boxes: []*Box{
			&Box{Coords: *r},
		},
	}
	f.RejectSize(0, 0, 200, 0)
	if !strings.Contains(f.Boxes[0].RejectReason, "width") {
		t.Errorf("want width rejectReason, got: %s", f.Boxes[0].RejectReason)
	}
	f.RejectSize(0, 0, 0, 200)
	if !strings.Contains(f.Boxes[0].RejectReason, "height") {
		t.Errorf("want height rejectReason, got: %s", f.Boxes[0].RejectReason)
	}
	f.RejectSize(400, 0, 0, 0)
	if !strings.Contains(f.Boxes[0].RejectReason, "width") {
		t.Errorf("want width rejectReason, got: %s", f.Boxes[0].RejectReason)
	}
	f.RejectSize(0, 400, 0, 0)
	if !strings.Contains(f.Boxes[0].RejectReason, "height") {
		t.Errorf("want height rejectReason, got: %s", f.Boxes[0].RejectReason)
	}
}
