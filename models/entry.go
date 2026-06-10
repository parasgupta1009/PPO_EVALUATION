package models

import "time"

type Entry struct {
	Value     string
	ExpiresAt time.Time
}

func (e Entry) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}
