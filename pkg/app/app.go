package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/marktheunissen/watchbot/pkg/camera"
	"github.com/marktheunissen/watchbot/pkg/detect"
	"github.com/marktheunissen/watchbot/pkg/frame"
	"github.com/marktheunissen/watchbot/pkg/jobs"
	"github.com/marktheunissen/watchbot/pkg/messaging"
	"github.com/marktheunissen/watchbot/pkg/systemerr"
	"github.com/marktheunissen/watchbot/pkg/utils"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("component", "app")

type Label struct {
	Name       string
	Confidence float64
}

type Config struct {
	Cams            []*camera.Cam
	Detector        *detect.Detector
	SysErr          *systemerr.LogReader
	PubSub          messaging.MessengerInterface
	FrameIntervalMS int
	HeartbeatURL    string
	SnapshotChan    chan jobs.Cmd
	FrameChan       chan jobs.Cmd
}

type App struct {
	Cams     []*camera.Cam
	Detector *detect.Detector
	SysErr   *systemerr.LogReader
	PubSub   messaging.MessengerInterface

	FrameInterval time.Duration
	StartupTime   time.Time
	HeartbeatURL  string
	CancelFn      context.CancelFunc
	RoundRobin    int

	AlertUploadChan chan *jobs.UploadJob
	InfoUploadChan  chan *jobs.UploadJob
	SnapshotChan    chan jobs.Cmd
	FrameChan       chan jobs.Cmd
	MsgChan         chan messaging.Msg
}

func New(config Config) (*App, error) {
	if config.FrameIntervalMS == 0 {
		config.FrameIntervalMS = 200
	}
	fi := time.Duration(config.FrameIntervalMS) * time.Millisecond
	for _, cam := range config.Cams {
		stats.cams = append(stats.cams, NewCamMetrics(cam.Name))
	}
	a := &App{
		Cams:            config.Cams,
		Detector:        config.Detector,
		SysErr:          config.SysErr,
		PubSub:          config.PubSub,
		FrameInterval:   fi,
		HeartbeatURL:    config.HeartbeatURL,
		AlertUploadChan: make(chan *jobs.UploadJob, 5000),
		InfoUploadChan:  make(chan *jobs.UploadJob, 5000),
		SnapshotChan:    config.SnapshotChan,
		FrameChan:       config.FrameChan,
		MsgChan:         make(chan messaging.Msg),
	}
	return a, nil
}

func (a *App) Run(ctx context.Context) error {
	a.StartupTime = time.Now().Round(time.Second)

	// Supervise frame rate, exit for restart if it drops
	go a.Supervisor(ctx)

	// Send critical errors to Telegram
	go a.ErrorPoller(ctx)

	// Command input from Telegram
	go a.ListenTelegram(ctx)

	// Uploader read
	go a.Uploader(ctx)

	// Heartbeat to the healthcheck alert service
	go a.Heartbeat(ctx)

	// PubSub message listener
	go a.MessengerListen(ctx)

	// PubSub message handler
	go a.HandleMessages(ctx)

	a.BotBroadcastMsg("App started")

	// Main loop, keep it in the main goroutine which is locked to thread.
	t := time.NewTicker(a.FrameInterval).C
	for {
		select {
		case <-t:
			stats.mainTicker.Mark(1)
			err := a.NextFrame()
			if err == detect.CamStatusInactive {
				log.Debug("Camera is not active yet")
				break
			}
			if err != nil {
				return err
			}

		case cmd := <-a.SnapshotChan:
			t := time.Now()
			jpegBytes, err := a.Detector.SnapshotNextFrame(cmd.CamIndex)
			if err == detect.CamStatusInactive {
				a.Cams[cmd.CamIndex].Bot.SendMsg("Camera feed is currently inactive, turn it on first")
				break
			}
			if err != nil {
				return err
			}
			a.InfoUploadChan <- &jobs.UploadJob{
				Caption:  fmt.Sprintf("Snapshot: %s", t.Format("2006-01-02 15:04:05.00")),
				Data:     bytes.NewBuffer(jpegBytes),
				CamIndex: cmd.CamIndex,
			}
			stats.cams[cmd.CamIndex].snapshot.Inc(1)

		case cmd := <-a.FrameChan:
			caption, jpegBytes, err := a.Detector.AnnotateNextFrame(cmd.CamIndex)
			if err == detect.CamStatusInactive {
				a.Cams[cmd.CamIndex].Bot.SendMsg("Camera feed is currently inactive, turn it on first")
				break
			}
			if err != nil {
				return err
			}
			a.InfoUploadChan <- &jobs.UploadJob{
				Caption:  caption,
				Data:     bytes.NewBuffer(jpegBytes),
				CamIndex: cmd.CamIndex,
			}
			stats.cams[cmd.CamIndex].snapshot.Inc(1)

		case <-ctx.Done():
			log.Info("App stopping")
			return nil
		}
	}
	return nil
}

func (a *App) Uploader(ctx context.Context) error {
	for {
		select {
		case job := <-a.AlertUploadChan:
			err := a.Cams[job.CamIndex].Bot.SendEvents([]string{job.Caption}, job.Data, true)
			if err != nil {
				log.Errorf("AlertChan Uploader SendEvents: %s", err)
				stats.cams[job.CamIndex].uploadError.Inc(1)
			} else {
				stats.cams[job.CamIndex].uploadSuccess.Inc(1)
			}
		case job := <-a.InfoUploadChan:
			err := a.Cams[job.CamIndex].Bot.SendEvents([]string{job.Caption}, job.Data, false)
			if err != nil {
				log.Errorf("InfoChan Uploader SendEvents: %s", err)
				stats.cams[job.CamIndex].uploadError.Inc(1)
			} else {
				stats.cams[job.CamIndex].uploadSuccess.Inc(1)
			}
		case <-ctx.Done():
			log.Info("Uploader stopping")
			return nil
		}
	}
	return nil
}

func (a *App) BotBroadcastMsg(msg string) {
	for _, cam := range a.Cams {
		cam.Bot.SendMsg(msg)
	}
}

func (a *App) Heartbeat(ctx context.Context) {
	if a.HeartbeatURL == "" {
		log.Info("No HeartbeatURL, not starting heartbeat")
		return
	}
	client := &http.Client{
		Timeout: 20 * time.Second,
	}
	makeReq := func() {
		stats.heartbeatTick.Inc(1)
		req, err := http.NewRequest("GET", a.HeartbeatURL, nil)
		if err != nil {
			log.Error("Heartbeat: %s", err)
			stats.heartbeatError.Inc(1)
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Error("Heartbeat: %s", err)
			stats.heartbeatError.Inc(1)
			return
		}
		if resp.StatusCode != 200 {
			log.Error("Heartbeat: got incorrect status code: %d", resp.StatusCode)
			stats.heartbeatError.Inc(1)
			return
		}
		resp.Body.Close()
	}
	// Run once on startup to fail fast if URL is unreachable
	makeReq()
	t := time.NewTicker(2 * time.Minute).C
	for {
		select {
		case <-t:
			makeReq()
		case <-ctx.Done():
			log.Info("Heartbeat stopping")
			return
		}
	}
}

func (a *App) NextFrame() error {
	a.RoundRobin += 1
	if a.RoundRobin >= len(a.Cams) {
		a.RoundRobin = 0
	}
	if a.Cams[a.RoundRobin].IsActive() {
		err := a.NextFrameFromCam(a.RoundRobin)
		if err != nil {
			return err
		}
		stats.frameRead.Mark(1)
	} else {
		log.Debugf("schedule for camera%d is off, skipping frame", a.RoundRobin)
		stats.frameSkip.Mark(1)
	}
	return nil
}

func (a *App) NextFrameFromCam(index int) error {
	cam := a.Cams[index]
	camStats := stats.cams[cam.Index]
	fdr, err := a.Detector.DetectNextFrame(cam.Index)
	if err == detect.CamStatusInactive {
		return err
	}
	if err != nil {
		camStats.detectorError.Inc(1)
		return err
	}
	if fdr == nil {
		log.Debug("nothing detected in frame")
		camStats.detectorNone.Inc(1)
		return nil
	}
	fdr.RejectSize(cam.MinWidth, cam.MinHeight, cam.MaxWidth, cam.MaxHeight)
	fdr.RejectOrientation(cam.RequirePortrait)
	fdr.RejectOutsideROI(cam.ROIRect)
	fdr.RejectLowConfidence(cam.MinConfidence)
	for _, box := range fdr.RejectedBoxes() {
		camStats.boxReject.Inc(1)
		if cam.SendRejected {
			a.InfoUploadChan <- &jobs.UploadJob{
				CamIndex: cam.Index,
				Caption:  box.RejectReason,
				Data:     bytes.NewBuffer(box.JPEGBytes),
			}
		}
	}
	if len(fdr.HitBoxes()) > 0 {
		camStats.detectorHit.Inc(1)
		log.Infof("camera%d (%s) detector hit", cam.Index, cam.Name)
		a.maybeSendOverview(cam.Index, bytes.NewBuffer(fdr.JPEGBytes))
		for _, box := range fdr.HitBoxes() {
			a.collectBoxStats(camStats, box)
			a.maybeSendBox(cam.Index, box.LabelConfidence(), bytes.NewBuffer(box.JPEGBytes))
		}
	}
	return nil
}

func (a *App) collectBoxStats(stats *CamMetrics, b *frame.Box) {
	stats.boxWidths.Inc(b.GetWidth())
	stats.boxHeights.Inc(b.GetHeight())
	stats.boxConfidences.Inc(b.Confidence)
}

func (a *App) maybeSendOverview(camIndex int, jpegBytes io.Reader) {
	cam := a.Cams[camIndex]
	camStats := stats.cams[camIndex]
	if time.Now().Sub(cam.LastOverviewSent) < 10*time.Second {
		camStats.overviewDrop.Inc(1)
		return
	}
	canContinue := cam.TakeFrameBuckets()
	if !canContinue {
		camStats.overviewDrop.Inc(1)
		return
	}
	cam.LastOverviewSent = time.Now()
	camStats.overviewSend.Inc(1)
	a.AlertUploadChan <- &jobs.UploadJob{
		CamIndex: camIndex,
		Caption:  "",
		Data:     jpegBytes,
	}
}

func (a *App) maybeSendBox(camIndex int, label string, jpegBytes io.Reader) {
	cam := a.Cams[camIndex]
	camStats := stats.cams[camIndex]
	canContinue := cam.TakeFrameBuckets()
	if !canContinue {
		camStats.boxDrop.Inc(1)
		return
	}
	camStats.boxSend.Inc(1)
	a.AlertUploadChan <- &jobs.UploadJob{
		CamIndex: camIndex,
		Caption:  label,
		Data:     jpegBytes,
	}
}

func (a *App) GetParams() string {
	out := fmt.Sprintf("FrameInterval: %v\n", a.FrameInterval)
	out += fmt.Sprintf("AlertLabels: %v\n", a.Detector.AlertLabels)
	return utils.MarkdownCode(out)
}

func (a *App) CancelAndExit() {
	a.CancelFn()
	go func() {
		time.Sleep(5 * time.Second)
		log.Warn("Hard exit")
		os.Exit(1)
	}()
}
