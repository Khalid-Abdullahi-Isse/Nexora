package http

import (
	"encoding/json"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/post-service/internal/service"
)

type CreatePostRequest struct {
	Content  *string `json:"content"`
	ImageURL *string `json:"image_url"`
}
type UpdatePostRequest struct {
	Content  json.RawMessage `json:"content"`
	ImageURL json.RawMessage `json:"image_url"`
}

func (r UpdatePostRequest) changes() (service.Changes, error) {
	var ch service.Changes
	if r.Content != nil {
		var content string
		if string(r.Content) == "null" || json.Unmarshal(r.Content, &content) != nil {
			return ch, service.ErrInvalid
		}
		ch.Content = &content
	}
	if r.ImageURL != nil {
		ch.ImageURLSet = true
		if json.Unmarshal(r.ImageURL, &ch.ImageURL) != nil {
			return ch, service.ErrInvalid
		}
	}
	return ch, nil
}
