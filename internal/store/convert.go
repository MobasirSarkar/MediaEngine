package store

import (
	"time"
)

func toPtrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func fromPtrTime(p *time.Time) time.Time {
	if p == nil {
		return time.Time{}
	}
	return *p
}

func toPtrStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func fromPtrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
