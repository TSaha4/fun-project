package domain

import (
	"strings"
	"time"
)

type Attachment struct {
	ID        string    `json:"id"`
	FileName  string    `json:"file_name"`
	MediaType string    `json:"media_type"`
	Size      int64     `json:"size"`
	Checksum  string    `json:"checksum"`
	CreatedAt time.Time `json:"created_at"`
}

func NewAttachment(id, fileName, mediaType string, size int64, checksum string, now time.Time) (Attachment, error) {
	a := Attachment{ID: id, FileName: strings.TrimSpace(fileName), MediaType: strings.TrimSpace(mediaType), Size: size, Checksum: checksum, CreatedAt: now}
	if strings.TrimSpace(id) == "" {
		return Attachment{}, ErrInvalidID
	}
	if a.FileName == "" || a.MediaType == "" || size < 0 || now.IsZero() {
		return Attachment{}, ErrInvalidAttachment
	}
	return a, nil
}
