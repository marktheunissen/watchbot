package detect

import (
	"testing"

	"github.com/marktheunissen/watchbot/pkg/camera"
	h "github.com/marktheunissen/watchbot/pkg/test/helpers"
)

func config() Config {
	camConf := camera.Config{
		Name:            "testCam",
		Index:           0,
		VideoCaptureURI: "./cartest.mp4",
	}
	c, err := camera.New(camConf)
	if err != nil {
		panic(err)
	}
	return Config{
		Cameras:       []*camera.Cam{c},
		GraphFileName: "/home/mark/Workarea/ncappzoo/caffe/SSD_MobileNet/graph",
		GraphLabels: []string{
			"background",
			"aeroplane",
			"bicycle",
			"bird",
			"boat",
			"bottle",
			"bus",
			"car",
			"cat",
			"chair",
			"cow",
			"dining table",
			"dog",
			"horse",
			"motorbike",
			"person",
			"potted plant",
			"sheep",
			"sofa",
			"train",
			"tvmonitor",
		},
		AlertLabels: []string{"person", "car"},
		GraphWidth:  300,
		GraphHeight: 300,
	}
}

func TestGrabFrame(t *testing.T) {
	d, err := New(config())
	h.FatalIfErr(t, err)
	fdr, err := d.DetectNextFrame(0)
	h.FatalIfErr(t, err)
	if len(fdr.Boxes) < 2 {
		t.Fatalf("Did not find 2 boxes in video frame")
	}
	for _, v := range fdr.Boxes {
		if v.Label != "car" {
			t.Fatalf("want label=car, got: %s", v.Label)
		}
	}
	d.Close()
}
