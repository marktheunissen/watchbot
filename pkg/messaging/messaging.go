package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
)

var log = logrus.WithField("component", "messaging")

// A Config stores the configuration for messaging Client instances
type Config struct {
	CredsPath      string
	TopicID        string
	SubscriptionID string
	ProjectID      string
}

// A Messenger provies the API for interacting with a pubsub messaging queue (google Pub/Sub)
type Messenger struct {
	PubsubClient   *pubsub.Client
	SubscriptionID string
	TopicID        string
	Subscription   *pubsub.Subscription
}

type MsgContents struct {
	Message string `json:"message"`
}
type Msg struct {
	Data MsgContents `json:"data"`
}

// New returns a new Messenger with provided configuration initialized and attached
func New(config Config) (*Messenger, error) {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, config.ProjectID, option.WithCredentialsFile(config.CredsPath))
	if err != nil {
		return nil, fmt.Errorf("Failed to create pubsub client: %v", err)
	}
	topic, err := getTopic(client, config.TopicID)
	if err != nil {
		return nil, fmt.Errorf("Topic error: %s", err)
	}
	subs, err := getSubscription(client, topic, config.SubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("Subscription error: %s", err)
	}
	log.Info("PubSub subscription initialized")

	return &Messenger{
		PubsubClient:   client,
		SubscriptionID: config.SubscriptionID,
		TopicID:        config.TopicID,
		Subscription:   subs,
	}, nil
}

func getSubscription(client *pubsub.Client, topic *pubsub.Topic, id string) (*pubsub.Subscription, error) {
	var err error
	subscription := client.Subscription(id)
	ctx := context.Background()
	ok, err := subscription.Exists(ctx)
	if err != nil {
		return nil, fmt.Errorf("Unable to determine if subscription '%s' exists: %v", id, err)
	}
	if !ok {
		subscription, err = client.CreateSubscription(ctx, id, pubsub.SubscriptionConfig{Topic: topic})
		if err != nil {
			return nil, fmt.Errorf("Failed to create subscription: %v", err)
		}
	}
	// A new goroutine is created internally for each message as it arrives, we want to
	// cap the total number of goroutines that can be spawned to not exhaust this app's resources.
	// This doesn't drop messages, it just stops pulling them until current ones are handled.
	subscription.ReceiveSettings.MaxOutstandingMessages = 100
	return subscription, nil
}

func getTopic(client *pubsub.Client, id string) (*pubsub.Topic, error) {
	topic := client.Topic(id)
	ctx := context.Background()
	ok, err := topic.Exists(ctx)
	if err != nil {
		return nil, fmt.Errorf("Unable to determine if topic '%s' exists: %v", id, err)
	}
	if !ok {
		return nil, fmt.Errorf("Topic '%s' does not exist", id)
	}
	return topic, nil
}

type MessengerInterface interface {
	RunListener(context.Context, chan<- Msg) error
	DeleteSub() error
}

func (m *Messenger) RunListener(ctx context.Context, msgChan chan<- Msg) error {
	return m.Subscription.Receive(ctx, func(ctx context.Context, psMsg *pubsub.Message) {
		var msg Msg
		err := json.Unmarshal(psMsg.Data, &msg)
		if err != nil {
			log.Errorf("Could not decode message data: %+v", string(psMsg.Data))
			psMsg.Ack()
			return
		}
		log.Infof("Message: %+v", msg)
		msgChan <- msg
		psMsg.Ack()
		log.Debugf("ACK: %+v", msg)
	})
}

// DeleteSub removes the subscription
func (m *Messenger) DeleteSub() error {
	log.Infof("Deleting subscription: %s", m.SubscriptionID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exists, err := m.Subscription.Exists(ctx)
	if err != nil {
		return err
	}
	if exists {
		err := m.Subscription.Delete(ctx)
		if err != nil {
			return err
		}
	}
	log.Infof("Subscription deleted: %s", m.SubscriptionID)
	return nil
}
