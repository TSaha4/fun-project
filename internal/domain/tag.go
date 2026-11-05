package domain

import (
	"strings"
	"time"
)

type Tag struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func NewTag(id, name string, now time.Time) (Tag, error) {
	t := Tag{ID: id, Name: strings.TrimSpace(name), CreatedAt: now}
	if strings.TrimSpace(id) == "" {
		return Tag{}, ErrInvalidID
	}
	if t.Name == "" {
		return Tag{}, ErrInvalidName
	}
	if now.IsZero() {
		return Tag{}, ErrInvalidTimestamp
	}
	return t, nil
}
