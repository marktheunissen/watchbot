package datastore

import (
	"bytes"
	"database/sql"
	"sync"

	"github.com/marktheunissen/watchbot/pkg/render"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("component", "datastore")

type ScheduleName int8

const (
	UploadSched ScheduleName = iota
)

type Config struct {
	Filename string
}

type Store struct {
	db          *sql.DB
	uploadSched *Schedule

	schedules map[ScheduleName]*Schedule

	// SQLite is not safe for concurrent write access.
	lock sync.Mutex
}

func New(config Config) (*Store, error) {
	db, err := sql.Open("sqlite3", config.Filename+"?_busy_timeout=2000")
	if err != nil {
		return nil, err
	}
	sqlStmt := `CREATE TABLE upload_sched (dayhour TEXT NOT NULL PRIMARY KEY);`
	db.Exec(sqlStmt)
	sqlStmt = `CREATE TABLE sched_mode (sched TEXT NOT NULL PRIMARY KEY, mode TEXT NOT NULL);`
	db.Exec(sqlStmt)
	s := &Store{
		db: db,
		uploadSched: &Schedule{
			Table: "upload_sched",
			Db:    db,
		},
		schedules: map[ScheduleName]*Schedule{},
	}
	s.schedules[UploadSched] = s.uploadSched
	return s, nil
}

func (s *Store) SchedActiveNow(sched ScheduleName) (bool, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.schedules[sched].IsActiveNow()
}

func (s *Store) SchedActivateAll(sched ScheduleName) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	err := s.schedules[sched].ActivateAll()
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) SchedDeactivateAll(sched ScheduleName) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	err := s.schedules[sched].DeactivateAll()
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) SchedGetTable(sched ScheduleName) (*bytes.Buffer, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	data, header, err := s.schedules[sched].GetTable()
	if err != nil {
		return nil, err
	}
	filebytes, err := render.TableJpeg(data, header)
	if err != nil {
		return nil, err
	}
	return filebytes, nil
}

func (s *Store) SchedActivate(sched ScheduleName, dayhour string) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	err := s.schedules[sched].Activate(dayhour)
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) SchedDeactivate(sched ScheduleName, dayhour string) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	err := s.schedules[sched].Deactivate(dayhour)
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) SchedSetMode(sched ScheduleName, mode ScheduleMode) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	err := s.schedules[sched].SetMode(mode)
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) SchedGetMode(sched ScheduleName) (ScheduleMode, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.schedules[sched].GetMode()
}

func (s *Store) Close() {
	s.db.Close()
}
