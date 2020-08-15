package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/marktheunissen/watchbot/pkg/app"
	"github.com/marktheunissen/watchbot/pkg/appmetrics"
	"github.com/marktheunissen/watchbot/pkg/camera"
	"github.com/marktheunissen/watchbot/pkg/datastore"
	"github.com/marktheunissen/watchbot/pkg/detect"
	"github.com/marktheunissen/watchbot/pkg/jobs"
	"github.com/marktheunissen/watchbot/pkg/messaging"
	"github.com/marktheunissen/watchbot/pkg/systemerr"
	"github.com/marktheunissen/watchbot/pkg/telegram"
	_ "github.com/mattn/go-sqlite3"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/wercker/journalhook"
)

const appName = "watchbot"

func init() {
	// Pins the main goroutine to a single thread so it will never be rescheduled.
	// This is especially desirable for CGO - could help to prevent RPi crashes.
	runtime.LockOSThread()
}

func initConfig() error {
	var err error
	viper.SetDefault("debug", false)
	viper.SetDefault("log-journal", true)
	viper.SetDefault("metrics-debug-port", 6060)
	viper.SetDefault("detect-graph-labels", []string{"background", "aeroplane", "bicycle", "bird", "boat", "bottle", "bus", "car", "cat", "chair", "cow", "dining table", "dog", "horse", "motorbike", "person", "potted plant", "sheep", "sofa", "train", "tvmonitor"})
	viper.SetDefault("detect-graph-width", 300)
	viper.SetDefault("detect-graph-height", 300)
	viper.SetDefault("detect-graph-file", "/opt/graph-mobilenet-sdk-1.0/graph")
	viper.SetDefault("frame-interval-ms", 200)
	viper.SetDefault("sqlite-db-dir", "/var/watchbot/")
	viper.SetDefault("active", true)

	dashToUs := strings.NewReplacer("-", "_")
	viper.SetConfigName(appName)                 // name of config file (without extension)
	viper.AddConfigPath(".")                     // cwd is highest (preferred) config path
	viper.AddConfigPath("$HOME")                 // home directory as second search path
	viper.AddConfigPath("/etc/watchbot")         // /etc/watchbot fallback
	viper.SetEnvPrefix(strings.ToUpper(appName)) // environment variable prefix
	viper.SetEnvKeyReplacer(dashToUs)            // convert environment variable keys from - to _
	viper.AutomaticEnv()                         // read in environment variables that match
	err = viper.ReadInConfig()
	if err != nil {
		return fmt.Errorf("config read error: %s", err)
	}
	return nil
}

func initLog() {
	log.SetLevel(log.InfoLevel)
	if viper.GetBool("debug") {
		log.SetLevel(log.DebugLevel)
	}
	log.SetOutput(os.Stdout)
	if viper.GetBool("log-journal") {
		journalhook.Enable()
	}
}

func initCam(viperConf *viper.Viper, index int) (*camera.Cam, error) {
	// Telegram bot first, so that we get a clue if stuck in crash loop due to
	// subsequent component failure.
	tgConfig := telegram.Config{
		Token:          viperConf.GetString("telegram-bot-token"),
		AlertGroupID:   viperConf.GetString("telegram-group-alert"),
		CommandGroupID: viperConf.GetString("telegram-group-command"),
		// Debug:           true,
	}
	bot, err := telegram.New(tgConfig)
	exitIfErr(err, "telegram.New")
	bot.SendMsg("🤖")

	// Datastore: SQLite DB
	dbFile := filepath.Clean(fmt.Sprintf("%s/camera%d.db", viper.GetString("sqlite-db-dir"), index))
	dsConfig := datastore.Config{
		Filename: dbFile,
	}
	store, err := datastore.New(dsConfig)
	exitIfErr(err, "datastore.New")

	camURI := strings.Replace(viperConf.GetString("pipeline"), "{url}", viperConf.GetString("url"), 1)
	if camURI == "" {
		return nil, errors.New("VideoCapture URL & pipeline are required")
	}

	config := camera.Config{
		Name:            viperConf.GetString("name"),
		Bot:             bot,
		Store:           store,
		Index:           index,
		VideoCaptureURI: camURI,
		CropX:           viperConf.GetInt("crop-x"),
		CropY:           viperConf.GetInt("crop-y"),
		CropWidth:       viperConf.GetInt("crop-w"),
		CropHeight:      viperConf.GetInt("crop-h"),
		MaxWidth:        viperConf.GetInt("max-width"),
		MaxHeight:       viperConf.GetInt("max-height"),
		MinWidth:        viperConf.GetInt("min-width"),
		MinHeight:       viperConf.GetInt("min-height"),
		MinConfidence:   viperConf.GetInt("min-confidence"),
		RequirePortrait: viperConf.GetBool("require-portrait"),
		ROIX:            viperConf.GetInt("roi-x"),
		ROIY:            viperConf.GetInt("roi-y"),
		ROIWidth:        viperConf.GetInt("roi-w"),
		ROIHeight:       viperConf.GetInt("roi-h"),
		SendRejected:    viperConf.GetBool("send-rejected"),
		PubSubControl:   viperConf.GetBool("pubsub-control"),
	}

	c, err := camera.New(config)
	if err != nil {
		return nil, err
	}
	log.Infof("camera%d (%s) Datastore Config: %+v", index, config.Name, dsConfig)
	log.Infof("camera%d (%s) Config: %+v", index, config.Name, config)
	return c, nil
}

func initMessagingClient() (messaging.MessengerInterface, error) {
	if viper.GetBool("pubsub-enable") {
		jsonCreds := viper.GetString("pubsub-creds-path")
		if jsonCreds == "" {
			return nil, errors.New("Required pubsub-creds-path not found in config")
		}
		subsID := fmt.Sprintf("%s-%s", viper.GetString("pubsub-subscription"), hostName())
		config := messaging.Config{
			CredsPath:      jsonCreds,
			ProjectID:      viper.GetString("pubsub-project"),
			SubscriptionID: subsID,
			TopicID:        viper.GetString("pubsub-topic"),
		}
		log.Infof("PubSub Messaging Client config loaded: %+v", config)
		return messaging.New(config)
	} else {
		return &messaging.NoopMessenger{}, nil
	}
}

func main() {
	err := initConfig()
	exitIfErr(err, "initConfig")
	initLog()

	// If inactive, we startup and pause, so that systemd doesn't kill the app
	// and eventually reboot, when it can't get it back.
	if !viper.GetBool("active") {
		log.Warn("Application disabled with 'active: false', pausing indefinitely")
		pause := make(chan bool, 1)
		<-pause
	}

	// Channels for communication between cameras and app
	SnapshotChan := make(chan jobs.Cmd, 30)
	FrameChan := make(chan jobs.Cmd, 30)

	// Cameras configuration
	var cams []*camera.Cam
	for i := 0; i < 4; i++ {
		camIndex := fmt.Sprintf("camera%d", i)
		conf := viper.Sub(camIndex)
		if conf != nil {
			c, err := initCam(conf, i)
			exitIfErr(err, camIndex+".initCam")
			cams = append(cams, c)
			c.SnapshotChan = SnapshotChan
			c.FrameChan = FrameChan
		} else {
			log.Infof("%s: no config found", camIndex)
		}
	}
	if len(cams) > 0 {
		log.Infof("Cameras initialized: %d", len(cams))
	} else {
		log.Fatal("No cameras found in config")
	}

	// Detector
	detectorConfig := detect.Config{
		Cameras:        cams,
		StickDeviceNum: viper.GetInt("detect-stick-device-num"),
		GraphFileName:  viper.GetString("detect-graph-file"),
		GraphLabels:    viper.GetStringSlice("detect-graph-labels"),
		AlertLabels:    viper.GetStringSlice("detect-alert-labels"),
		GraphWidth:     viper.GetInt("detect-graph-width"),
		GraphHeight:    viper.GetInt("detect-graph-height"),
	}
	detector, err := detect.New(detectorConfig)
	exitIfErr(err, "detector.New")
	log.Infof("Detector Config: %+v", detectorConfig)

	// Metrics configuration
	metricsConfig := appmetrics.Config{
		DebugBindPort:  viper.GetInt("metrics-debug-port"),
		DebugBindAddr:  "localhost",
		GraphiteHost:   viper.GetString("graphite-host"),
		MetricHostname: viper.GetString("metric-hostname"),
		AppName:        appName,
		FlushInterval:  time.Second * 1,
	}
	err = appmetrics.Run(metricsConfig)
	exitIfErr(err, "metrics.Run")
	log.Infof("Metrics config: %+v", metricsConfig)

	// Error logs from the system
	sysErrConf := systemerr.Config{}
	sysErr, err := systemerr.New(sysErrConf)
	exitIfErr(err, "systemerror.New")
	log.Infof("SystemErr config: %+v", sysErrConf)

	// PubSub Messaging client
	messagingClient, err := initMessagingClient()
	exitIfErr(err, "initMessagingClient")

	// App configuration
	appConfig := app.Config{
		Cams:            cams,
		Detector:        detector,
		SysErr:          sysErr,
		PubSub:          messagingClient,
		FrameIntervalMS: viper.GetInt("frame-interval-ms"),
		HeartbeatURL:    viper.GetString("heartbeat-url"),
		SnapshotChan:    SnapshotChan,
		FrameChan:       FrameChan,
	}
	a, err := app.New(appConfig)
	exitIfErr(err, "app.New")
	log.Infof("App Config: %+v", appConfig)

	// Signal handling
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		signal.Stop(c)
		cancel()
	}()
	go func() {
		select {
		case <-c:
			a.CancelAndExit()
		case <-ctx.Done():
		}
	}()

	a.CancelFn = cancel
	log.Debug("Starting app with a.Run()")
	err = a.Run(ctx)
	exitIfErr(err, "app.Run")
	detector.Close()
	for _, cam := range a.Cams {
		log.Infof("Cleanup camera%d", cam.Index)
		cam.Store.Close()
	}
	log.Info("Shutdown complete")
}

func exitIfErr(err error, msg string) {
	if err != nil {
		log.Fatal(msg + ": " + err.Error())
	}
}

func hostName() string {
	hs, err := os.Hostname()
	if err != nil {
		hs = "unknown"
	}
	return hs
}
