package messaging

import (
	"context"
)

type NoopMessenger struct{}

func (m *NoopMessenger) RunListener(ctx context.Context, msgChan chan<- Msg) error {
	log.Info("PubSub disabled, not listening for any PubSub messages")
	return nil
}

func (m *NoopMessenger) DeleteSub() error {
	return nil
}
