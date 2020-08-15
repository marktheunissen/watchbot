package systemerr

import (
	"io"
	"time"

	"github.com/coreos/go-systemd/sdjournal"
)

type Config struct {
}

type LogReader struct {
	Journal *sdjournal.JournalReader
}

func New(config Config) (*LogReader, error) {
	l := &LogReader{}
	err := l.Reset()
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (l *LogReader) Reset() error {
	if l.Journal != nil {
		l.Journal.Close()
	}
	jrConf := sdjournal.JournalReaderConfig{
		Matches: []sdjournal.Match{
			{
				Field: "PRIORITY",
				Value: "3",
			},
			{
				Field: "PRIORITY",
				Value: "2",
			},
			{
				Field: "PRIORITY",
				Value: "1",
			},
		},
		Since:     -60 * time.Minute,
		Formatter: getMsg,
	}
	var err error
	l.Journal, err = sdjournal.NewJournalReader(jrConf)
	return err
}

func (l *LogReader) NextError() (string, error) {
	data := make([]byte, 64*1<<(10)) // 64K, copied from the "Follow" code in sdjournal.go
	count, err := l.Journal.Read(data)
	if err != nil && err != io.EOF {
		return "", err
	}
	if count > 0 {
		return string(data[:count]), nil
	}
	return "", nil
}

func getMsg(entry *sdjournal.JournalEntry) (string, error) {
	if msg, ok := entry.Fields["MESSAGE"]; ok {
		return msg, nil
	}
	return "Unknown error, no MESSAGE", nil
}
