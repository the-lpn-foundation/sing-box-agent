// Package validators provides request validation for API endpoints.
package validators

import (
	"encoding/json"
	"errors"
)

// InboundCreateRequest represents a create inbound request.
type InboundCreateRequest struct {
	Tag     string                 `json:"tag"`
	Type    string                 `json:"type"`
	Listen  string                 `json:"listen"`
	Options map[string]interface{} `json:"options"`
}

// UserCreateRequest represents a create user request.
type UserCreateRequest struct {
	SubID        string `json:"subId"`
	Email        string `json:"email"`
	TrafficLimit int64  `json:"trafficLimit"`
	Expiry       int64  `json:"expiry"`
}

// ValidateInboundCreate validates an inbound creation request.
func ValidateInboundCreate(data []byte) error {
	var req InboundCreateRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}
	if req.Tag == "" {
		return errors.New("tag is required")
	}
	if req.Type == "" {
		return errors.New("type is required")
	}
	if req.Listen == "" {
		return errors.New("listen is required")
	}
	return nil
}

// ValidateUserCreate validates a user creation request.
func ValidateUserCreate(data []byte) error {
	var req UserCreateRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}
	if req.SubID == "" {
		return errors.New("subId is required")
	}
	return nil
}
