package datastore_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/marktheunissen/watchbot/pkg/datastore"
	h "github.com/marktheunissen/watchbot/pkg/test/helpers"
)

func getDS(t *testing.T) (*datastore.Store, func()) {
	tmpfile, err := ioutil.TempFile("", "test-dbs")
	t.Log("opening new sqlitedb: " + tmpfile.Name())
	config := datastore.Config{
		Filename: tmpfile.Name(),
	}
	d, err := datastore.New(config)
	h.FatalIfErr(t, err)

	cleanup := func() {
		t.Log("cleaning up " + tmpfile.Name())
		os.Remove(tmpfile.Name())
	}
	return d, cleanup
}

func TestSchedule(t *testing.T) {
	d, cleanup := getDS(t)
	defer cleanup()

	mode, err := d.SchedGetMode(datastore.UploadSched)
	h.FatalIfErr(t, err)
	if mode != datastore.ModeSched {
		t.Fatalf("expected default mode of scheduled, got: %s", datastore.ModeStr(mode))
	}

	err = d.SchedSetMode(datastore.UploadSched, datastore.ModeOn)
	h.FatalIfErr(t, err)
	mode, err = d.SchedGetMode(datastore.UploadSched)
	h.FatalIfErr(t, err)
	if mode != datastore.ModeOn {
		t.Fatalf("expected mode change to On, got: %s", datastore.ModeStr(mode))
	}

	err = d.SchedSetMode(datastore.UploadSched, datastore.ModeOff)
	h.FatalIfErr(t, err)
	mode, err = d.SchedGetMode(datastore.UploadSched)
	h.FatalIfErr(t, err)
	if mode != datastore.ModeOff {
		t.Fatalf("expected mode change to On, got: %s", datastore.ModeStr(mode))
	}

	err = d.SchedSetMode(datastore.UploadSched, datastore.ModeSched)
	h.FatalIfErr(t, err)

	active, err := d.SchedActiveNow(datastore.UploadSched)
	h.FatalIfErr(t, err)
	if active {
		t.Fatal("expected inactive schedule, found active")
	}

	err = d.SchedActivateAll(datastore.UploadSched)
	h.FatalIfErr(t, err)
	active, err = d.SchedActiveNow(datastore.UploadSched)
	h.FatalIfErr(t, err)
	if !active {
		t.Fatal("expected active schedule, found inactive")
	}

	err = d.SchedDeactivateAll(datastore.UploadSched)
	h.FatalIfErr(t, err)
	active, err = d.SchedActiveNow(datastore.UploadSched)
	h.FatalIfErr(t, err)
	if active {
		t.Fatal("expected inactive schedule, found active")
	}
}
