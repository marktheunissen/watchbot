package app

import (
	"context"
	"fmt"
	"time"

	"github.com/marktheunissen/watchbot/pkg/utils"
)

func (a *App) ErrorPoller(ctx context.Context) {
	a.reportErrors()
	t := time.NewTicker(59 * time.Minute).C
	for {
		select {
		case <-t:
			a.reportErrors()

		case <-ctx.Done():
			log.Info("ErrorPoller stopping")
			return
		}
	}
}

// reportErrors will send errors found in the journal to Telegram. Will send at most X errors at a time.
func (a *App) reportErrors() {
	err := a.SysErr.Reset()
	if err != nil {
		msg := fmt.Sprintf("SysErr.Reset: %s", err)
		a.BotBroadcastMsg(msg)
		log.Error(msg)
		return
	}
	errsFound := map[string]int{}
	for i := 0; i < 1000; i++ {
		entry, err := a.SysErr.NextError()
		if err != nil {
			msg := fmt.Sprintf("SysErr.NextError: %s", err)
			a.BotBroadcastMsg(msg)
			log.Error(msg)
			return
		}
		if entry != "" {
			_, ok := errsFound[entry]
			if ok {
				errsFound[entry]++
			} else {
				errsFound[entry] = 1
			}
		}
	}
	log.Debugf("Found the following errors to scan: %+v", errsFound)

	searchStrings := []string{"Under-voltage"}
	for k, v := range errsFound {
		if utils.StringPrefixInSlice(k, searchStrings) {
			a.BotBroadcastMsg(fmt.Sprintf("❗️Errors (%d): %s", v, k))
		}
	}
}
