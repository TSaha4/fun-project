package domain

import (
	"strings"
	"time"
)

type Collection struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewCollection(id, name string, now time.Time) (Collection, error) {
	c := Collection{ID: id, Name: strings.TrimSpace(name), CreatedAt: now, UpdatedAt: now}
	if strings.TrimSpace(id) == "" {
		return Collection{}, ErrInvalidID
	}
	if c.Name == "" {
		return Collection{}, ErrInvalidName
	}
	if now.IsZero() {
		return Collection{}, ErrInvalidTimestamp
	}
	return c, nil
}
