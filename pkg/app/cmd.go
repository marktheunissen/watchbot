package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/marktheunissen/watchbot/pkg/datastore"
	"github.com/marktheunissen/watchbot/pkg/jobs"
)

var helpText = `
(bot or b)
bot ping
bot snap
bot frame
bot params
bot metrics
bot uptime
bot tokens
bot hists
bot isactive
bot restart
bot sched get
bot sched init
bot sched on <hour-day|day|hour,...> e.g. mon-0, mon, 0
bot sched off <hour-day|day|hour,...>
bot mode get
bot mode set <on|off|sched>
`

func (a *App) ListenTelegram(ctx context.Context) {
	cmds := make(chan jobs.Cmd, 10000)
	for i, _ := range a.Cams {
		go a.Cams[i].Bot.PollUpdatesToChan(i, cmds)
	}

	log := log.WithField("function", "listenTelegram")
	for {
		select {
		case cmd := <-cmds:
			a.handleIncomingCmd(cmd)
		case <-ctx.Done():
			log.Info("Telegram stopping")
			return
		}
	}
}

func (a *App) handleIncomingCmd(cmd jobs.Cmd) {
	log.Infof("Got cmd: %v", cmd)
	c := a.Cams[cmd.CamIndex]
	if cmd.Noun == "help" {
		c.Bot.SendMsg(helpText)
	}
	if cmd.Noun == "ping" {
		c.Bot.SendMsg("pong")
	}
	if cmd.Noun == "snap" {
		c.SnapshotChan <- cmd
	}
	if cmd.Noun == "frame" {
		c.FrameChan <- cmd
	}
	if cmd.Noun == "tokens" {
		c.Bot.SendMsg(c.TokensRemaining())
	}
	if cmd.Noun == "params" {
		c.Bot.SendMsg(a.GetParams())
		c.Bot.SendMsg(c.GetParams())
	}
	if cmd.Noun == "restart" {
		a.CancelAndExit()
	}
	if cmd.Noun == "hists" {
		i := cmd.CamIndex
		err := a.SendHist(i, stats.cams[i].boxHeights)
		if err != nil {
			log.Errorf("Hists: %s", err)
		}
		err = a.SendHist(i, stats.cams[i].boxWidths)
		if err != nil {
			log.Errorf("Hists: %s", err)
		}
		err = a.SendHist(i, stats.cams[i].boxConfidences)
		if err != nil {
			log.Errorf("Hists: %s", err)
		}
	}
	if cmd.Noun == "isactive" {
		active, err := c.Store.SchedActiveNow(datastore.UploadSched)
		if err != nil {
			log.Errorf("SchedActiveNow: %s", err)
			return
		}
		c.Bot.SendMsg(fmt.Sprintf("IsActive: %v", active))
	}
	if cmd.Noun == "metrics" {
		c.Bot.SendMsg(MetricsPrintOut())
	}
	if cmd.Noun == "uptime" {
		c.Bot.SendMsg(fmt.Sprintf("Uptime: %s", time.Now().Round(time.Second).Sub(a.StartupTime)))
	}
	if cmd.Noun == "sched" {
		if cmd.Verb == "init" {
			err := c.Store.SchedActivateAll(datastore.UploadSched)
			if err != nil {
				log.Errorf("Sched init: %s", err)
			}
			filebytes, err := c.Store.SchedGetTable(datastore.UploadSched)
			if err != nil {
				log.Errorf("Sched get: %s", err)
			} else {
				c.Bot.SendImageBytesBuf(filebytes)
			}
		}
		if cmd.Verb == "get" {
			filebytes, err := c.Store.SchedGetTable(datastore.UploadSched)
			if err != nil {
				log.Errorf("Sched get: %s", err)
			} else {
				c.Bot.SendImageBytesBuf(filebytes)
			}
		}
		if cmd.Verb == "on" {
			if cmd.Obj == "" {
				c.Bot.SendMsg("usage: bot sched on <hour-day|day|hour,...>")
				return
			}
			for _, o := range strings.Split(cmd.Obj, ",") {
				err := c.Store.SchedActivate(datastore.UploadSched, strings.TrimSpace(o))
				if err != nil {
					log.Errorf("Sched set: %s", err)
				}
			}
			filebytes, err := c.Store.SchedGetTable(datastore.UploadSched)
			if err != nil {
				log.Errorf("Sched get: %s", err)
			} else {
				c.Bot.SendImageBytesBuf(filebytes)
			}
		}
		if cmd.Verb == "off" {
			if cmd.Obj == "" {
				c.Bot.SendMsg("usage: bot sched off <hour-day|day|hour,...>")
				return
			}
			for _, o := range strings.Split(cmd.Obj, ",") {
				err := c.Store.SchedDeactivate(datastore.UploadSched, strings.TrimSpace(o))
				if err != nil {
					log.Errorf("Sched set: %s", err)
				}
			}
			filebytes, err := c.Store.SchedGetTable(datastore.UploadSched)
			if err != nil {
				log.Errorf("Sched get: %s", err)
			} else {
				c.Bot.SendImageBytesBuf(filebytes)
			}
		}
	}
	if cmd.Noun == "mode" {
		if cmd.Verb == "get" {
			currMode, err := c.Store.SchedGetMode(datastore.UploadSched)
			if err != nil {
				log.Errorf("Mode get: %s", err)
			} else {
				c.Bot.SendMsg(fmt.Sprintf("Mode set to '%s'", datastore.ModeStr(currMode)))
			}
		}
		if cmd.Verb == "set" {
			mode := datastore.StrMode(cmd.Obj)
			if mode == datastore.ModeInvalid {
				c.Bot.SendMsg("usage: bot mode <on|off|sched>")
				return
			}
			err := c.Store.SchedSetMode(datastore.UploadSched, mode)
			if err != nil {
				log.Errorf("Mode set: %s", err)
			} else {
				newMode, err := c.Store.SchedGetMode(datastore.UploadSched)
				if err != nil {
					log.Errorf("Mode get: %s", err)
				} else {
					c.Bot.SendMsg(fmt.Sprintf("Mode set to '%s'", datastore.ModeStr(newMode)))
				}
			}
		}
	}
}
