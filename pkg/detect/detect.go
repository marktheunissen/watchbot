package detect

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io/ioutil"
	"math"
	"strings"
	"sync"
	"time"

	ncs "github.com/hybridgroup/go-ncs"
	"github.com/marktheunissen/watchbot/pkg/camera"
	"github.com/marktheunissen/watchbot/pkg/frame"
	"github.com/mewmew/floats/binary16"
	"github.com/sirupsen/logrus"
	"gocv.io/x/gocv"
)

var log = logrus.WithField("component", "detect")

var CamStatusInactive = errors.New("Camera is inactive")

var bgColor = color.RGBA{0, 215, 0, 0}

type Config struct {
	Cameras        []*camera.Cam
	StickDeviceNum int
	GraphFileName  string
	GraphLabels    []string
	AlertLabels    []string
	GraphWidth     int
	GraphHeight    int
}

type Detector struct {
	Cameras      []*camera.Cam
	Captures     []*gocv.VideoCapture
	CapturesLock sync.Mutex
	Graph        *ncs.Graph
	GraphLabels  []string
	AlertLabels  []string
	NCSDevice    *ncs.Stick
	Frame        gocv.Mat
	FrameRaw     gocv.Mat
	FrameResized gocv.Mat
	FrameFP32    gocv.Mat
	GraphWidth   int
	GraphHeight  int
	SubMat       gocv.Mat
	MulMat       gocv.Mat
}

func New(config Config) (*Detector, error) {
	log.Infof("GoCV version: %s", gocv.Version())
	log.Infof("OpenCV lib version: %s", gocv.OpenCVVersion())

	// Connect to Neural Compute Stick, load graph file
	result, name := ncs.GetDeviceName(config.StickDeviceNum)
	if result != ncs.StatusOK {
		return nil, fmt.Errorf("NCS GetDeviceName #%d, status: %v=%s", config.StickDeviceNum, int(result), ncsStatusToText(int(result)))
	}
	log.Infof("Opened Neural Compute Stick: %s", strings.TrimRight(name, "\x00"))

	status, ncsDevice := ncs.OpenDevice(name)
	if status != ncs.StatusOK {
		return nil, fmt.Errorf("NCS OpenDevice %s: %v", name, status)
	}

	data, err := ioutil.ReadFile(config.GraphFileName)
	if err != nil {
		return nil, fmt.Errorf("Open Graph File: %s", err)
	}

	allocateStatus, graph := ncsDevice.AllocateGraph(data)
	if allocateStatus != ncs.StatusOK {
		return nil, fmt.Errorf("NCS AllocateGraph: %v", allocateStatus)
	}

	if config.GraphWidth == 0 || config.GraphHeight == 0 {
		return nil, errors.New("Graph height and width must be > 0")
	}
	if len(config.AlertLabels) == 0 {
		log.Warn("no alert labels given to detector, will not alert on anything")
	}
	if len(config.GraphLabels) == 0 {
		return nil, errors.New("Graph labels are required")
	}

	d := &Detector{
		Cameras:      config.Cameras,
		Captures:     make([]*gocv.VideoCapture, len(config.Cameras)),
		Graph:        graph,
		NCSDevice:    ncsDevice,
		GraphLabels:  config.GraphLabels,
		AlertLabels:  config.AlertLabels,
		Frame:        gocv.NewMat(),
		FrameRaw:     gocv.NewMat(),
		FrameResized: gocv.NewMat(),
		FrameFP32:    gocv.NewMat(),
		GraphHeight:  config.GraphHeight,
		GraphWidth:   config.GraphWidth,

		// 21 is the mat.Type(), which matches what was read from the type of Mat that resulted
		// from converting to 32bit float (d.FrameFP32.Type())
		SubMat: gocv.NewMatWithSizeFromScalar(gocv.NewScalar(127, 127, 127, 127), config.GraphWidth, config.GraphHeight, 21),
		MulMat: gocv.NewMatWithSizeFromScalar(gocv.NewScalar(0.007843, 0.007843, 0.007843, 0.007843), config.GraphWidth, config.GraphHeight, 21),
	}
	return d, nil
}

func (d *Detector) Close() {
	for _, c := range d.Captures {
		if c != nil {
			c.Close()
		}
	}
	d.Graph.DeallocateGraph()
	d.NCSDevice.CloseDevice()
}

// Turn on and off camera feeds depending on whether they are active or not
func (d *Detector) ToggleFeeds() error {
	d.CapturesLock.Lock()
	defer d.CapturesLock.Unlock()
	for i, cam := range d.Cameras {
		if cam.IsActive() && d.Captures[i] == nil {
			err := d.startCamFeed(i)
			if err != nil {
				return err
			}
		} else if !cam.IsActive() && d.Captures[i] != nil {
			err := d.stopCamFeed(i)
			if err != nil {
				return err
			}
		} else {
			continue
		}

		// Just a hunch that opening these capture feeds in rapid succession is causing hanging.
		time.Sleep(time.Second * 3)
	}
	return nil
}

func (d *Detector) startCamFeed(i int) error {
	vc, err := gocv.VideoCaptureFile(d.Cameras[i].VideoCaptureURI)
	if err != nil {
		return fmt.Errorf("startCamFeed open camera%d: %s", i, err)
	}
	log.Infof("Opened video device camera%d: %s", i, d.Cameras[i].VideoCaptureURI)
	d.Captures[i] = vc
	return nil
}

func (d *Detector) stopCamFeed(i int) error {
	err := d.Captures[i].Close()
	d.Captures[i] = nil
	if err != nil {
		return fmt.Errorf("stopCamFeed close camera%d: %s", i, err)
	}
	log.Infof("Closed video device camera%d: %s", i, d.Cameras[i].VideoCaptureURI)
	return nil
}

func (d *Detector) CamRead(camIndex int, frame *gocv.Mat) error {
	d.CapturesLock.Lock()
	defer d.CapturesLock.Unlock()
	if d.Captures[camIndex] == nil {
		return CamStatusInactive
	}
	ok := d.Captures[camIndex].Read(frame)
	if !ok {
		return fmt.Errorf("Cam.Read camera%d: no frame", camIndex)
	}
	if frame.Empty() {
		return fmt.Errorf("Cam.Read camera%d: empty frame", camIndex)
	}
	log.Debugf("camera%d: read a frame size: %d x %d", camIndex, frame.Cols(), frame.Rows())
	return nil
}

func (d *Detector) SnapshotNextFrame(camIndex int) ([]byte, error) {
	frame := gocv.NewMat()
	err := d.CamRead(camIndex, &frame)
	if err != nil {
		return nil, err
	}
	defer frame.Close()

	frameJPEG, err := gocv.IMEncode(".jpg", frame)
	if err != nil {
		return nil, fmt.Errorf("IMEncode frame camera%d: %s", camIndex, err)
	}
	return frameJPEG, nil
}

func (d *Detector) AnnotateNextFrame(camIndex int) (string, []byte, error) {
	roiRect := d.Cameras[camIndex].ROIRect
	frame := gocv.NewMat()
	err := d.CamRead(camIndex, &frame)
	if err != nil {
		return "", nil, err
	}
	defer frame.Close()

	overlay := gocv.NewMat()
	frame.CopyTo(&overlay)

	msgROI := "none"
	msgCrop := "none"
	cropRect := d.Cameras[camIndex].CropRect
	if cropRect != nil {
		cropColor := color.RGBA{255, 0, 0, 0}
		gocv.Rectangle(&overlay, *cropRect, cropColor, 2)
		msgCrop = fmt.Sprintf("%v %dx%d", cropRect, cropRect.Dx(), cropRect.Dy())

		if roiRect != nil {
			roiDraw := image.Rect(cropRect.Min.X+roiRect.Min.X, cropRect.Min.Y+roiRect.Min.Y, cropRect.Min.X+roiRect.Max.X, cropRect.Min.Y+roiRect.Max.Y)
			roiColor := color.RGBA{0, 0, 255, 0}
			gocv.Rectangle(&overlay, roiDraw, roiColor, 2)
			msgROI = fmt.Sprintf("%v %dx%d", roiRect, roiRect.Dx(), roiRect.Dy())
		}
	}
	alpha := 0.5
	gocv.AddWeighted(overlay, alpha, frame, 1-alpha, 0, &frame)

	caption := fmt.Sprintf("Frame %dx%d, crop: %s, roi: %s", frame.Cols(), frame.Rows(), msgCrop, msgROI)
	frameJPEG, err := gocv.IMEncode(".jpg", frame)
	if err != nil {
		return "", nil, fmt.Errorf("IMEncode frame: %s", err)
	}
	return caption, frameJPEG, nil
}

func (d *Detector) DetectNextFrame(camIndex int) (*frame.FrameDetectResult, error) {
	err := d.CamRead(camIndex, &d.FrameRaw)
	if err != nil {
		return nil, err
	}

	// Crop image if directed, which can give a better detection result if the aspect ratio is 1:1
	cropRect := d.Cameras[camIndex].CropRect
	if cropRect != nil {
		// .Region creates a new object in C++, need to delete it. Cropping cannot be
		// configured mid-execution so this should be safe as it will be recreated new iteration.
		d.Frame = d.FrameRaw.Region(*cropRect)
		defer d.Frame.Close()
	} else {
		d.Frame = d.FrameRaw
	}
	// Debugging tools
	// gocv.IMWrite("test-run-image-a.jpg", d.Frame)
	// log.Infof("1. frame: %v, frameresized: %v, framefp32: %v", d.Frame.Type(), d.FrameResized.Type(), d.FrameFP32.Type())

	// Convert image to format needed by NCS
	// This model requires us to resize the 300x300 expected, then
	// transform pixel values from range (0-255) to (-1.0 - 1.0)
	// In Python this is simple due to overloaded operator:
	//    resized_image = resized_image - 127.5
	//    resized_image = resized_image * 0.007843
	gocv.Resize(d.Frame, &d.FrameResized, image.Pt(d.GraphWidth, d.GraphHeight), 0, 0, gocv.InterpolationArea)
	d.FrameResized.ConvertTo(&d.FrameFP32, gocv.MatTypeCV32F)
	gocv.Subtract(d.FrameFP32, d.SubMat, &d.FrameFP32)
	gocv.Multiply(d.FrameFP32, d.MulMat, &d.FrameFP32)
	fp16Blob := d.FrameFP32.ConvertFp16()
	defer fp16Blob.Close()

	// Load image tensor into graph on NCS stick
	loadStatus := d.Graph.LoadTensor(fp16Blob.ToBytes())
	if loadStatus != ncs.StatusOK {
		return nil, fmt.Errorf("LoadTensor: %v", loadStatus)
	}

	// Get result from NCS stick in fp16 format
	resultStatus, data := d.Graph.GetResult()
	if resultStatus != ncs.StatusOK {
		return nil, fmt.Errorf("LoadTensor: %v", resultStatus)
	}

	fdr := &frame.FrameDetectResult{}
	fdr.Boxes, err = d.ParseResult(data)
	if err != nil {
		return nil, fmt.Errorf("ParseResult: %s", err)
	}

	fdr.ParseAlerts(d.AlertLabels)
	if len(fdr.Boxes) == 0 {
		return nil, nil
	}

	for _, box := range fdr.Boxes {
		err := d.makeCrop(box)
		if err != nil {
			return nil, fmt.Errorf("makeCrop: %s", err)
		}
	}
	for _, box := range fdr.Boxes {
		// Do overlay after making the crop to avoid cropping intersecting box lines.
		gocv.Rectangle(&d.Frame, box.Coords, bgColor, 2)
	}

	fdr.JPEGBytes, err = gocv.IMEncode(".jpg", d.Frame)
	if err != nil {
		return nil, fmt.Errorf("IMEncode frame: %s", err)
	}
	return fdr, nil
}

func (d *Detector) makeCrop(box *frame.Box) error {
	roi := d.Frame.Region(box.Coords)
	defer roi.Close()
	var err error
	box.JPEGBytes, err = gocv.IMEncode(".jpg", roi)
	if err != nil {
		return err
	}
	return nil
}

//   a.	First fp16 value holds the number of valid detections = num_valid.
//   b.	The next 6 values are unused.
//   c.	The next (7 * num_valid) values contain the valid detections data
//       Each group of 7 values will describe an object/box These 7 values in order.
//       The values are:
//         0: image_id (always 0)
//         1: class_id (this is an index into labels)
//         2: score (this is the probability for the class)
//         3: box left location within image as number between 0.0 and 1.0
//         4: box top location within image as number between 0.0 and 1.0
//         5: box right location within image as number between 0.0 and 1.0
//         6: box bottom location within image as number between 0.0 and 1.0
func (d *Detector) ParseResult(data []byte) ([]*frame.Box, error) {
	var header = struct {
		Count  uint16
		Unused [6]uint16
	}{}
	buf := bytes.NewReader(data)
	err := binary.Read(buf, binary.LittleEndian, &header)
	if err != nil {
		return nil, fmt.Errorf("binary.Read: %s", err)
	}
	numBoxes := int(floatFromBits(header.Count))

	retBoxes := []*frame.Box{}
	var boxdata = struct {
		ImageID uint16
		ClassID uint16
		Score   uint16
		Left    uint16
		Top     uint16
		Right   uint16
		Bottom  uint16
	}{}
	for i := 0; i < numBoxes; i++ {
		err := binary.Read(buf, binary.LittleEndian, &boxdata)
		if err != nil {
			return nil, fmt.Errorf("binary.Read: %s", err)
		}
		imageID := floatFromBits(boxdata.ImageID)
		classID := floatFromBits(boxdata.ClassID)
		score := floatFromBits(boxdata.Score)
		left := floatFromBits(boxdata.Left)
		top := floatFromBits(boxdata.Top)
		right := floatFromBits(boxdata.Right)
		bottom := floatFromBits(boxdata.Bottom)
		if invalidFloat(imageID) || invalidFloat(classID) || invalidFloat(score) || invalidFloat(left) || invalidFloat(top) || invalidFloat(right) || invalidFloat(bottom) {
			// According to NCS specs, it's desirable to continue here if any floats are invalid
			continue
		}
		if int(classID) > len(d.GraphLabels)-1 || int(classID) < 0 {
			// Label value returned in the result doesn't match anything we know about,
			// it's out of range
			continue
		}

		X1 := actualPos(left, d.Frame.Cols())
		Y1 := actualPos(top, d.Frame.Rows())
		X2 := actualPos(right, d.Frame.Cols())
		Y2 := actualPos(bottom, d.Frame.Rows())
		b := &frame.Box{
			Label:      d.GraphLabels[int(classID)],
			Confidence: int(score * 100),
			Coords:     image.Rect(X1, Y1, X2, Y2),
		}
		log.Debugf("found box: %+v", b)
		retBoxes = append(retBoxes, b)
	}
	return retBoxes, nil
}

func invalidFloat(f float64) bool {
	return math.IsInf(f, 0) || math.IsNaN(f)
}

func actualPos(frac float64, d int) int {
	// Boxes can be negative, so set them to border.
	p := int(frac * float64(d))
	if p < 0 {
		p = 0
	}
	if p > d {
		p = d
	}
	return p
}

func floatFromBits(bits uint16) float64 {
	f := binary16.NewFromBits(bits)
	return f.Float64()
}
