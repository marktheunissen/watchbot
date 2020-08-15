package helpers

import (
	"reflect"
	"testing"
)

func FatalIfErr(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

func TestStats(t *testing.T, s reflect.Value) {
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		if !f.IsNil() {
			t.Logf("%s looks correct", s.Type().Field(i).Name)
		} else {
			t.Fatalf("%s is set to nil, this will panic", s.Type().Field(i).Name)
		}
	}
}
