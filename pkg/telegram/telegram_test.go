package telegram_test

import (
	"os"
	"testing"

	"github.com/marktheunissen/watchbot/pkg/telegram"
	h "github.com/marktheunissen/watchbot/pkg/test/helpers"
)

func TestTelegram(t *testing.T) {
	config := telegram.Config{
		Token:          "mytesttoken",
		CommandGroupID: "-12345",
		AlertGroupID:   "-12345678",
	}
	bot, err := telegram.New(config)
	h.FatalIfErr(t, err)

	f, err := os.Open("../../test-fixtures/images/pika.jpg")
	h.FatalIfErr(t, err)

	err = bot.SendEvents([]string{"Alert!"}, f, true)

	f, err = os.Open("../../test-fixtures/images/pika.jpg")
	h.FatalIfErr(t, err)

	err = bot.SendEvents([]string{"Command response"}, f, false)
	h.FatalIfErr(t, err)
}
