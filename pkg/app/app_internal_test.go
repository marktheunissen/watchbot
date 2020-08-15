package app

import (
	"reflect"
	"testing"
)

// TestEnsureStats ensures that the stats are all correctly non-nil.
func TestEnsureStats(t *testing.T) {
	s := reflect.ValueOf(stats).Elem()
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		if !f.IsNil() {
			t.Logf("%s looks correct", s.Type().Field(i).Name)
		} else {
			t.Fatalf("%s is set to nil, this will panic", s.Type().Field(i).Name)
		}
	}
}
