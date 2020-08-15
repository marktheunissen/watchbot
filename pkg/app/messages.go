package app

import (
	"context"
	"strings"

	"github.com/marktheunissen/watchbot/pkg/jobs"
	"github.com/marktheunissen/watchbot/pkg/messaging"
)

// MessengerListen invokes the messenger pubsub interface to listen for incoming messages
func (a *App) MessengerListen(ctx context.Context) error {
	log.Info("Starting MessengerListen")
	err := a.PubSub.RunListener(ctx, a.MsgChan)
	if ctx.Err() != nil {
		// The context is cancelled, we can shutdown
		log.Info("PubSub messenger stopping")
		errSubsDel := a.PubSub.DeleteSub()
		return errSubsDel
	}
	return err
}

// HandleMessages pulls incoming messages from the messenger (pubsub) and handles them.
func (a *App) HandleMessages(ctx context.Context) {
	for {
		select {
		case msg := <-a.MsgChan:
			log.Debugf("App message from queue: %+v", msg)
			a.handleMessage(ctx, msg)

		case <-ctx.Done():
			return
		}
	}
}

// handleMessage just processes a single message.
func (a *App) handleMessage(ctx context.Context, msg messaging.Msg) {
	log.Info("Handling message")
	log.Infof("Got message: %+v", msg)
	if strings.Contains(msg.Data.Message, "HOUSE ALARM") {
		if strings.Contains(msg.Data.Message, "OPENING") {
			for _, c := range a.Cams {
				if c.PubSubControl {
					c := jobs.Cmd{
						CamIndex: c.Index,
						Noun:     "mode",
						Verb:     "set",
						Obj:      "off",
					}
					a.handleIncomingCmd(c)
				}
			}
		} else if strings.Contains(msg.Data.Message, "CLOSE") {
			for _, c := range a.Cams {
				if c.PubSubControl {
					c := jobs.Cmd{
						CamIndex: c.Index,
						Noun:     "mode",
						Verb:     "set",
						Obj:      "on",
					}
					a.handleIncomingCmd(c)
				}
			}
		}
	}
	log.Info("Message handling complete")
}
