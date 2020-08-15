package datastore

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ScheduleMode int

const (
	ModeInvalid ScheduleMode = 0
	ModeSched   ScheduleMode = 1
	ModeOn      ScheduleMode = 2
	ModeOff     ScheduleMode = 3
)

func ModeStr(mode ScheduleMode) string {
	switch mode {
	case ModeSched:
		return "Sched"
	case ModeOn:
		return "On"
	case ModeOff:
		return "Off"
	}
	return ""
}

func StrMode(mode string) ScheduleMode {
	switch strings.ToLower(mode) {
	case "sched":
		return ModeSched
	case "on":
		return ModeOn
	case "off":
		return ModeOff
	}
	return ModeInvalid
}

var Weekdays = []time.Weekday{
	time.Monday,
	time.Tuesday,
	time.Wednesday,
	time.Thursday,
	time.Friday,
	time.Saturday,
	time.Sunday,
}

func TimeToDayHourString(t time.Time) string {
	return DayHourToString(t.Weekday(), t.Hour())
}

func DayHourToString(day time.Weekday, hour int) string {
	return fmt.Sprintf("%s-%d", day.String(), hour)
}

type Schedule struct {
	Table string
	Db    *sql.DB
}

func (s *Schedule) DeactivateAll() error {
	_, err := s.Db.Exec("DELETE FROM " + s.Table)
	if err != nil {
		return err
	}
	return nil
}

func (s *Schedule) ActivateAll() error {
	for _, day := range Weekdays {
		for hour := 0; hour < 24; hour++ {
			err := s.SetActive(day, hour)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Schedule) SetActive(day time.Weekday, hour int) error {
	_, err := s.Db.Exec("INSERT OR REPLACE INTO "+s.Table+" (dayhour) VALUES ($1)", DayHourToString(day, hour))
	if err != nil {
		return err
	}
	return nil
}

func (s *Schedule) SetDeactive(day time.Weekday, hour int) error {
	_, err := s.Db.Exec("DELETE FROM "+s.Table+" WHERE dayhour = ($1)", DayHourToString(day, hour))
	if err != nil {
		return err
	}
	return nil
}

func (s *Schedule) IsActiveNow() (bool, error) {
	sm, err := s.GetMode()
	if err != nil {
		return false, err
	}
	if sm == ModeOn {
		return true, nil
	}
	if sm == ModeOff {
		return false, nil
	}
	return s.IsActiveTime(time.Now())
}

func (s *Schedule) IsActiveTime(t time.Time) (bool, error) {
	var count int
	err := s.Db.QueryRow("SELECT count(*) FROM "+s.Table+" WHERE dayhour = $1", TimeToDayHourString(t)).Scan(&count)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	return false, nil
}

func (s *Schedule) IsActiveDayHour(day time.Weekday, hour int) (bool, error) {
	var count int
	err := s.Db.QueryRow("SELECT count(*) FROM "+s.Table+" WHERE dayhour = $1", DayHourToString(day, hour)).Scan(&count)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	return false, nil
}

func (s *Schedule) SetMode(mode ScheduleMode) error {
	_, err := s.Db.Exec("INSERT OR REPLACE INTO sched_mode (sched, mode) VALUES ($1, $2)", s.Table, ModeStr(mode))
	if err != nil {
		return err
	}
	return nil
}

func (s *Schedule) GetMode() (ScheduleMode, error) {
	var sm ScheduleMode
	var modeStr string
	err := s.Db.QueryRow("SELECT mode FROM sched_mode WHERE sched = $1", s.Table).Scan(&modeStr)
	if err == sql.ErrNoRows {
		smErr := s.SetMode(ModeSched)
		if smErr != nil {
			return sm, smErr
		}
		return ModeSched, nil
	}
	if err != nil {
		return sm, err
	}
	return StrMode(modeStr), nil
}

func StrToWeekday(day string) *time.Weekday {
	wd := new(time.Weekday)
	switch day {
	case "mon":
		*wd = time.Monday
	case "tue":
		*wd = time.Tuesday
	case "wed":
		*wd = time.Wednesday
	case "thu":
		*wd = time.Thursday
	case "fri":
		*wd = time.Friday
	case "sat":
		*wd = time.Saturday
	case "sun":
		*wd = time.Sunday
	default:
		wd = nil
	}
	return wd
}

func ParseDayHour(dayhour string) (*time.Weekday, int, error) {
	// Formats:
	// mon-0 to mon-23
	// mon
	// 4
	var err error
	var hourInt int

	if len(dayhour) < 1 {
		return nil, 0, errors.New("No input given")
	}
	dayhour = strings.TrimSpace(strings.ToLower(dayhour))
	split := strings.Split(dayhour, "-")

	if len(split) == 2 {
		if hourInt, err = strconv.Atoi(split[1]); err != nil {
			return nil, 0, fmt.Errorf("Invalid day or hour: %s", dayhour)
		}
		wd := StrToWeekday(split[0])
		if wd == nil {
			return nil, 0, fmt.Errorf("Invalid day: %s", split[0])
		}
		return wd, hourInt, nil
	}

	if len(split) == 1 {
		if hourInt, err := strconv.Atoi(split[0]); err == nil {
			return nil, hourInt, nil
		} else {
			wd := StrToWeekday(split[0])
			if wd == nil {
				return nil, 0, fmt.Errorf("Invalid day: %s", split[0])
			}
			return wd, -1, nil
		}
	}

	return nil, 0, fmt.Errorf("Invalid day or hour: %s", dayhour)
}

func (s *Schedule) Deactivate(dayhour string) error {
	day, hour, err := ParseDayHour(dayhour)
	if err != nil {
		return err
	}
	if hour == -1 && day == nil {
		return errors.New("No day or hour found")
	}
	if hour < -1 || hour > 23 {
		return errors.New("Invalid hour, only 0-23 allowed")
	}
	if hour == -1 {
		for h := 0; h < 24; h++ {
			err := s.SetDeactive(*day, h)
			if err != nil {
				return err
			}
		}
		return nil
	}
	if day == nil {
		for _, d := range Weekdays {
			err := s.SetDeactive(d, hour)
			if err != nil {
				return err
			}
		}
		return nil
	}
	err = s.SetDeactive(*day, hour)
	if err != nil {
		return err
	}
	return nil
}

func (s *Schedule) Activate(dayhour string) error {
	day, hour, err := ParseDayHour(dayhour)
	if err != nil {
		return err
	}
	if hour == -1 && day == nil {
		return errors.New("No day or hour found")
	}
	if hour == -1 {
		for h := 0; h < 24; h++ {
			err := s.SetActive(*day, h)
			if err != nil {
				return err
			}
		}
		return nil
	}
	if day == nil {
		for _, d := range Weekdays {
			err := s.SetActive(d, hour)
			if err != nil {
				return err
			}
		}
		return nil
	}
	err = s.SetActive(*day, hour)
	if err != nil {
		return err
	}
	return nil
}

func (s *Schedule) GetTable() ([][]string, []string, error) {
	header := []string{"Hour", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	data := [][]string{}

	for hour := 0; hour < 24; hour++ {
		hourStr := fmt.Sprintf("%d:00", hour)
		if hour < 10 {
			hourStr = "0" + hourStr
		}
		row := []string{hourStr}
		for _, day := range Weekdays {
			active, err := s.IsActiveDayHour(day, hour)
			if err != nil {
				return nil, nil, err
			}
			if active {
				row = append(row, "✔")
			} else {
				row = append(row, "")
			}
		}
		data = append(data, row)
	}
	return data, header, nil
}
