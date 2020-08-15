package telegram

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/marktheunissen/watchbot/pkg/jobs"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("component", "telegram")

type Config struct {
	Token          string
	AlertGroupID   string
	CommandGroupID string
	Debug          bool
}

type Bot struct {
	AlertGroupID   string
	CommandGroupID string
	tgBot          *tgbotapi.BotAPI
}

func New(config Config) (*Bot, error) {
	if config.AlertGroupID == "" || config.CommandGroupID == "" {
		return nil, errors.New("Missing alert or command group id")
	}
	tgBot, err := tgbotapi.NewBotAPI(config.Token)
	if err != nil {
		return nil, err
	}
	tgBot.Debug = config.Debug
	log.Infof("Telegram authorized on account %s", tgBot.Self.UserName)
	bot := &Bot{
		tgBot:          tgBot,
		AlertGroupID:   config.AlertGroupID,
		CommandGroupID: config.CommandGroupID,
	}
	return bot, nil
}

func (b *Bot) PollUpdatesToChan(camIndex int, c chan jobs.Cmd) {
	config := tgbotapi.UpdateConfig{
		Offset:  0,
		Limit:   100,
		Timeout: 30,
	}
	for {
		log := log.WithField("function", "telegramCmdChan")
		log.Debug("Polling Telegram API for bot commands")
		updates, err := b.tgBot.GetUpdates(config)
		if err != nil {
			log.Errorf("Failed to get updates, retrying in 5 seconds: %s", err)
			time.Sleep(time.Second * 5)
			continue
		}

		if len(updates) > 0 {
			log.Infof("Got %d updates", len(updates))
		}
		for _, update := range updates {
			if update.UpdateID >= config.Offset {
				config.Offset = update.UpdateID + 1
				cmd := b.UpdateToCmd(update)
				cmd.CamIndex = camIndex
				c <- cmd
			}
		}
	}
}

func (b *Bot) UpdateToCmd(update tgbotapi.Update) jobs.Cmd {
	var cmd jobs.Cmd
	if update.Message == nil {
		return cmd
	}
	if fmt.Sprintf("%d", update.Message.Chat.ID) != b.CommandGroupID {
		log.Debugf("skipping message for chat ID %d since it's not in our chatroom", update.Message.Chat.ID)
		return cmd
	}
	pieces := strings.SplitN(strings.TrimSpace(update.Message.Text), " ", 4)
	if len(pieces) == 0 {
		return cmd
	}
	if strings.ToLower(pieces[0]) != "bot" && strings.ToLower(pieces[0]) != "b" {
		return cmd
	}
	if len(pieces) > 1 {
		cmd.Noun = strings.ToLower(strings.TrimSpace(pieces[1]))
	}
	if len(pieces) > 2 {
		cmd.Verb = strings.ToLower(strings.TrimSpace(pieces[2]))
	}
	if len(pieces) > 3 {
		cmd.Obj = strings.TrimSpace(pieces[3])
	}
	return cmd
}

// SendEvents sends pics to Telegram
func (b *Bot) SendEvents(labels []string, data io.Reader, isAlert bool) error {
	fr := tgbotapi.FileReader{
		Name:   "Event",
		Reader: data,
		Size:   -1,
	}
	conf := tgbotapi.NewPhotoUpload(int64(0), fr)

	// It actually only uses this conf.BaseChat.ChannelUsername. Secret channels
	// start with `-`, and just using the int64 causes an error that it can't find it.
	if isAlert {
		conf.BaseChat.ChannelUsername = b.AlertGroupID
	} else {
		conf.BaseChat.ChannelUsername = b.CommandGroupID
	}
	conf.Caption = ""
	for _, l := range labels {
		conf.Caption = conf.Caption + l + "\n"
	}
	capt := strings.TrimSpace(strings.Replace(conf.Caption, "\n", " ", -1))
	msg := "Sending overview image"
	if capt != "" {
		msg = fmt.Sprintf("Sending image with caption: '%s'", capt)
	}
	log.Info(msg)
	_, err := b.tgBot.Send(conf)
	return err
}

// SendMsg sends a message
func (b *Bot) SendMsg(msg string) error {
	conf := tgbotapi.NewMessage(int64(0), msg)
	conf.ParseMode = tgbotapi.ModeMarkdown
	conf.BaseChat.ChannelUsername = b.CommandGroupID
	msg = strings.Replace(msg, "\n", " ", -1)
	log.Infof("Sending: %s", msg)
	_, err := b.tgBot.Send(conf)
	return err
}

func (b *Bot) SendImageBytesBuf(imgBytes *bytes.Buffer) error {
	fb := tgbotapi.FileBytes{
		Name:  "image.jpg",
		Bytes: imgBytes.Bytes(),
	}
	conf := tgbotapi.NewPhotoUpload(int64(0), fb)
	conf.BaseChat.ChannelUsername = b.CommandGroupID
	_, err := b.tgBot.Send(conf)
	return err
}

func (b *Bot) SendImageBytesBufCaption(label string, data io.Reader) error {
	fb := tgbotapi.FileReader{
		Name:   "image.jpg",
		Reader: data,
		Size:   -1,
	}
	conf := tgbotapi.NewPhotoUpload(int64(0), fb)
	conf.BaseChat.ChannelUsername = b.CommandGroupID
	conf.Caption = label
	_, err := b.tgBot.Send(conf)
	return err
}
