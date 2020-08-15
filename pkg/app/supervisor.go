package app

import (
	"context"
	"fmt"
	"time"

	"github.com/marktheunissen/watchbot/pkg/datastore"
)

func (a *App) Supervisor(ctx context.Context) {
	t := time.NewTicker(5 * time.Second).C
	for {
		select {
		case <-t:
			stats.supervisorTick.Inc(1)
			a.checkModeSwitch()
			a.checkMainTicker()

		case <-ctx.Done():
			log.Info("Supervisor stopping")
			return
		}
	}
}

func (a *App) checkMainTicker() {
	// Ensure the stream hasn't crashed, shutdown the app if it has.
	ss := stats.mainTicker.Snapshot()
	if ss.Count() > 600 && ss.Rate1() < 2 {
		log.Infof("1 min: %.2f", ss.Rate1())
		log.Infof("5 min: %.2f", ss.Rate5())
		msg := fmt.Sprintf("Frame rate too low: %.1f, halting!", ss.Rate1())
		log.Error(msg)
		a.BotBroadcastMsg(msg)
		a.CancelAndExit()
	}
}

func (a *App) checkModeSwitch() {
	// Emit a message if the detector is switching mode, set the control bit
	// on the camera object.
	stateChanged := false
	for _, cam := range a.Cams {
		active, err := cam.Store.SchedActiveNow(datastore.UploadSched)
		if err != nil {
			log.Errorf("SchedActiveNow error: %s", err)
			break
		}
		if active != cam.IsActive() {
			cam.SetActive(active)
			if active {
				cam.Bot.SendMsg("Detector changed state: ON")
			} else {
				cam.Bot.SendMsg("Detector changed state: OFF")
			}
			stateChanged = true
		}
	}
	if stateChanged {
		log.Info("A camera has changed status, toggling the capture feed")
		err := a.Detector.ToggleFeeds()
		if err != nil {
			log.Errorf("checkModeSwitch error: %s", err)
		}
	}
}
