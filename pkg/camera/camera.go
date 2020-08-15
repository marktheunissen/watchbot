package camera

import (
	"fmt"
	"image"
	"strings"
	"sync"
	"time"

	"github.com/juju/ratelimit"
	"github.com/marktheunissen/watchbot/pkg/datastore"
	"github.com/marktheunissen/watchbot/pkg/jobs"
	"github.com/marktheunissen/watchbot/pkg/telegram"
	"github.com/marktheunissen/watchbot/pkg/utils"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("component", "camera")

type Config struct {
	Name            string
	Index           int
	Bot             *telegram.Bot
	Store           *datastore.Store
	VideoCaptureURI string
	PubSubControl   bool
	MaxWidth        int
	MaxHeight       int
	MinWidth        int
	MinHeight       int
	RequirePortrait bool
	MinConfidence   int
	SendRejected    bool
	CropX           int
	CropY           int
	CropWidth       int
	CropHeight      int
	ROIX            int
	ROIY            int
	ROIWidth        int
	ROIHeight       int
}

type Cam struct {
	Name             string
	Index            int
	Bot              *telegram.Bot
	Store            *datastore.Store
	FrameLimit       *ratelimit.Bucket
	BurstLimit       *ratelimit.Bucket
	PaceLimit        *ratelimit.Bucket
	LastOverviewSent time.Time
	SnapshotChan     chan jobs.Cmd
	FrameChan        chan jobs.Cmd
	PubSubControl    bool
	MaxWidth         int
	MaxHeight        int
	MinWidth         int
	MinHeight        int
	RequirePortrait  bool
	MinConfidence    int
	CropRect         *image.Rectangle
	ROIRect          *image.Rectangle
	SendRejected     bool
	VideoCaptureURI  string

	// Whether or not this camera should be active according to the schedule
	active     bool
	activeLock sync.Mutex
}

func New(config Config) (*Cam, error) {
	cropRect := utils.GetRectForROI(config.CropX, config.CropY, config.CropWidth, config.CropHeight)
	roiRect := utils.GetRectForROI(config.ROIX, config.ROIY, config.ROIWidth, config.ROIHeight)
	if roiRect != nil {
		if roiRect.Dx() > cropRect.Dx() || roiRect.Dy() > cropRect.Dy() {
			return nil, fmt.Errorf("ROI: %v must be smaller and relative to the crop image: %v", roiRect, cropRect)
		}
	}
	if config.MinConfidence == 0 {
		config.MinConfidence = 15
	}
	c := &Cam{
		Name:  config.Name,
		Index: config.Index,
		Store: config.Store,
		Bot:   config.Bot,

		// Rate limit for the bot, as per Telegram docs: https://core.telegram.org/bots/faq
		// 20 / min to same group
		// 1 / sec recommended rate with short bursts allowed.B
		// Send no more than 20 frames in total per minute. HaPd upper limit of Telegram.
		FrameLimit: ratelimit.NewBucketWithQuantum(1*time.Minute, 20, 20),

		// Send no more than 3 images in a burst, by refilling a bucket of 3 with 1 per sec.
		// This also mostly keeps us at 1/sec max when constant incoming stream.
		// Will use up all 20 after 17 seconds if not for the subsequent limit
		BurstLimit: ratelimit.NewBucketWithQuantum(1*time.Second, 3, 1),

		// Try not use up all the quota in the first 17 seconds.
		PaceLimit: ratelimit.NewBucketWithQuantum(15*time.Second, 6, 6),

		LastOverviewSent: time.Now().Add(-10 * time.Second),

		MinConfidence:   config.MinConfidence,
		MaxWidth:        config.MaxWidth,
		MaxHeight:       config.MaxHeight,
		MinWidth:        config.MinWidth,
		MinHeight:       config.MinHeight,
		RequirePortrait: config.RequirePortrait,
		CropRect:        cropRect,
		ROIRect:         roiRect,

		SendRejected: config.SendRejected,

		VideoCaptureURI: config.VideoCaptureURI,
		PubSubControl:   config.PubSubControl,
	}
	return c, nil
}

func (c *Cam) TakeFrameBuckets() bool {
	fl := c.FrameLimit.TakeAvailable(1)
	if fl != 1 {
		log.Debug("frameLimit reached")
		return false
	}
	bl := c.BurstLimit.TakeAvailable(1)
	if bl != 1 {
		log.Debug("burstLimit reached")
		return false
	}
	pl := c.PaceLimit.TakeAvailable(1)
	if pl != 1 {
		log.Debug("paceLimit reached")
		return false
	}
	log.Debugf("remaining tokens: frameLimit: %d, burstLimit: %d, paceLimit: %d", c.FrameLimit.Available(), c.BurstLimit.Available(), c.PaceLimit.Available())
	return true
}

func (c *Cam) GetParams() string {
	uri := "error"
	before := strings.Split(c.VideoCaptureURI, "://")
	after := strings.Split(c.VideoCaptureURI, "@")
	if len(before) == 2 || len(after) == 2 {
		uri = before[0] + "://" + after[1]
	}
	out := fmt.Sprintf("Name: %s\n", c.Name)
	out += fmt.Sprintf("Index: %d\n", c.Index)
	out += fmt.Sprintf("MaxWidth: %d\n", c.MaxWidth)
	out += fmt.Sprintf("MaxHeight: %d\n", c.MaxHeight)
	out += fmt.Sprintf("MinWidth: %d\n", c.MinWidth)
	out += fmt.Sprintf("MinHeight: %d\n", c.MinHeight)
	out += fmt.Sprintf("MinConfidence: %d\n", c.MinConfidence)
	out += fmt.Sprintf("RequirePortrait: %v\n", c.RequirePortrait)
	out += fmt.Sprintf("Crop: %+v\n", c.CropRect)
	out += fmt.Sprintf("ROI: %+v\n", c.ROIRect)
	out += fmt.Sprintf("Capture: %s\n", uri)
	out += fmt.Sprintf("SendRejected: %v\n", c.SendRejected)
	return utils.MarkdownCode(out)
}

func (c *Cam) TokensRemaining() string {
	out := fmt.Sprintf("frame: %d\n", c.FrameLimit.Available())
	out += fmt.Sprintf("burst: %d\n", c.BurstLimit.Available())
	out += fmt.Sprintf("pace: %d\n", c.PaceLimit.Available())
	out += fmt.Sprintf("overview: %v\n", time.Now().Sub(c.LastOverviewSent))
	return utils.MarkdownCode(out)
}

func (c *Cam) SetActive(active bool) {
	c.activeLock.Lock()
	defer c.activeLock.Unlock()
	c.active = active
}

func (c *Cam) IsActive() bool {
	c.activeLock.Lock()
	defer c.activeLock.Unlock()
	return c.active
}
