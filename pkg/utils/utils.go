package utils

import (
	"fmt"
	"image"
	"strings"
	"time"
)

// Round out a duration for printing
func Round(d, r time.Duration) time.Duration {
	if r <= 0 {
		return d
	}
	neg := d < 0
	if neg {
		d = -d
	}
	if m := d % r; m+m < r {
		d = d - m
	} else {
		d = d + r - m
	}
	if neg {
		return -d
	}
	return d
}

func GetRectForROI(x, y, w, h int) *image.Rectangle {
	if w < 20 || h < 20 {
		return nil
	}
	return GetRect(x, y, w, h)
}

func GetRect(x, y, w, h int) *image.Rectangle {
	r := image.Rect(x, y, x+w, y+h)
	return &r
}

func MarkdownCode(s string) string {
	return fmt.Sprintf("```\n%s```\n", s)
}

func StringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func StringPrefixInSlice(a string, list []string) bool {
	for _, b := range list {
		if strings.HasPrefix(a, b) {
			return true
		}
	}
	return false
}
