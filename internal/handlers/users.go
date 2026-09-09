package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/the-lpn-foundation/sing-box-agent/internal/models"
	syncpkg "github.com/the-lpn-foundation/sing-box-agent/internal/sync"
)

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Success bool `json:"success"`
	Error   struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// SuccessResponse represents a success response.
type SuccessResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
}

// UserHandler handles user CRUD operations.
type UserHandler struct {
	singbox syncpkg.SingBoxClient
}

// NewUserHandlerWithClient creates a new user handler backed by a sing-box client.
func NewUserHandlerWithClient(singbox syncpkg.SingBoxClient) *UserHandler {
	return &UserHandler{singbox: singbox}
}

// UserRequest represents a user creation/update request.
type UserRequest struct {
	SubID         string `json:"subID,omitempty"`
	UUID          string `json:"uuid,omitempty"`
	Email         string `json:"email,omitempty"`
	Enabled       *bool  `json:"enabled,omitempty"`
	Flow          string `json:"flow,omitempty"`
	LimitIP       *int   `json:"limitIp,omitempty"`
	UploadLimit   uint64 `json:"uploadLimit,omitempty"`
	DownloadLimit uint64 `json:"downloadLimit,omitempty"`
}

// Validate validates the user request.
func (r *UserRequest) Validate() error {
	if r.SubID == "" {
		return errors.New("subId is required")
	}
	if r.Email != "" && !strings.Contains(r.Email, "@") {
		return errors.New("invalid email format")
	}

	if r.LimitIP != nil && *r.LimitIP < 0 {
		return errors.New("limitIp cannot be negative")
	}
	return nil
}

// ToUser converts the request to a User model.
func (r *UserRequest) ToUser(inboundTag string) models.User {
	user := models.User{
		SubID:         r.SubID,
		UUID:          r.UUID,
		InboundTag:    inboundTag,
		Email:         r.Email,
		Flow:          r.Flow,
		UploadLimit:   r.UploadLimit,
		DownloadLimit: r.DownloadLimit,
		Enabled:       true, // default
	}

	if r.UUID == "" {
		// Generate UUID if not provided
		user.UUID = uuid.New().String()
	}

	if r.Enabled != nil {
		user.Enabled = *r.Enabled
	}

	if r.LimitIP != nil {
		user.LimitIP = *r.LimitIP
	}

	return user
}

// writeError writes an error response.
func writeError(w http.ResponseWriter, code int, errCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	resp := ErrorResponse{
		Success: false,
	}
	resp.Error.Code = errCode
	resp.Error.Message = message

	_ = json.NewEncoder(w).Encode(resp)
}

// writeSuccess writes a success response.
func writeSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	resp := SuccessResponse{
		Success: true,
		Data:    data,
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// writeCreated writes a created response.
func writeCreated(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	resp := SuccessResponse{
		Success: true,
		Data:    data,
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// ListUsers handles GET /inbounds/{tag}/users.
func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if h.singbox == nil {
		writeError(w, http.StatusServiceUnavailable, "INTERNAL_ERROR", "sing-box client not configured")
		return
	}

	inboundTag := extractInboundTag(r.URL.Path)
	if inboundTag == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "inbound tag is required")
		return
	}

	if !h.inboundExists(r, inboundTag) {
		writeError(w, http.StatusNotFound, "INBOUND_NOT_FOUND", fmt.Sprintf("inbound %s not found", inboundTag))
		return
	}

	users, err := h.singbox.GetUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", fmt.Sprintf("failed to list users: %v", err))
		return
	}

	filtered := make([]models.User, 0)
	for _, user := range users {
		if user.InboundTag == inboundTag {
			filtered = append(filtered, user)
		}
	}

	writeSuccess(w, filtered)
}

// GetUser handles GET /inbounds/{tag}/users/{subId}.
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	if h.singbox == nil {
		writeError(w, http.StatusServiceUnavailable, "INTERNAL_ERROR", "sing-box client not configured")
		return
	}

	inboundTag, subID := extractInboundTagAndSubID(r.URL.Path)
	if inboundTag == "" || subID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "inbound tag and subID are required")
		return
	}

	if !h.inboundExists(r, inboundTag) {
		writeError(w, http.StatusNotFound, "INBOUND_NOT_FOUND", fmt.Sprintf("inbound %s not found", inboundTag))
		return
	}

	users, err := h.singbox.GetUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", fmt.Sprintf("failed to list users: %v", err))
		return
	}

	for _, user := range users {
		if user.InboundTag == inboundTag && user.SubID == subID {
			writeSuccess(w, user)
			return
		}
	}

	writeError(w, http.StatusNotFound, "USER_NOT_FOUND", fmt.Sprintf("user %s not found", subID))
}

// CreateUser handles POST /inbounds/{tag}/users.
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if h.singbox == nil {
		writeError(w, http.StatusServiceUnavailable, "INTERNAL_ERROR", "sing-box client not configured")
		return
	}

	inboundTag := extractInboundTag(r.URL.Path)
	if inboundTag == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "inbound tag is required")
		return
	}

	if !h.inboundExists(r, inboundTag) {
		writeError(w, http.StatusNotFound, "INBOUND_NOT_FOUND", fmt.Sprintf("inbound %s not found", inboundTag))
		return
	}

	var req UserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	user := req.ToUser(inboundTag)
	if err := h.singbox.CreateUser(r.Context(), user); err != nil {
		h.mapClientError(w, err)
		return
	}

	writeCreated(w, user)
}

// UpdateUser handles PUT /inbounds/{tag}/users/{subId}.
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	if h.singbox == nil {
		writeError(w, http.StatusServiceUnavailable, "INTERNAL_ERROR", "sing-box client not configured")
		return
	}

	inboundTag, subID := extractInboundTagAndSubID(r.URL.Path)
	if inboundTag == "" || subID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "inbound tag and subID are required")
		return
	}

	if !h.inboundExists(r, inboundTag) {
		writeError(w, http.StatusNotFound, "INBOUND_NOT_FOUND", fmt.Sprintf("inbound %s not found", inboundTag))
		return
	}

	var req UserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if req.SubID != "" && req.SubID != subID {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "subId in body must match subID in path")
		return
	}
	req.SubID = subID

	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.UUID == "" {
		users, err := h.singbox.GetUsers(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", fmt.Sprintf("failed to list users: %v", err))
			return
		}
		for _, existing := range users {
			if existing.InboundTag == inboundTag && existing.SubID == subID {
				req.UUID = existing.UUID
				break
			}
		}
	}

	user := req.ToUser(inboundTag)
	if err := h.singbox.UpdateUser(r.Context(), user); err != nil {
		h.mapClientError(w, err)
		return
	}

	writeSuccess(w, user)
}

// DeleteUser handles DELETE /inbounds/{tag}/users/{subId}.
func (h *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if h.singbox == nil {
		writeError(w, http.StatusServiceUnavailable, "INTERNAL_ERROR", "sing-box client not configured")
		return
	}

	inboundTag, subID := extractInboundTagAndSubID(r.URL.Path)
	if inboundTag == "" || subID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "inbound tag and subID are required")
		return
	}

	if !h.inboundExists(r, inboundTag) {
		writeError(w, http.StatusNotFound, "INBOUND_NOT_FOUND", fmt.Sprintf("inbound %s not found", inboundTag))
		return
	}

	users, err := h.singbox.GetUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", fmt.Sprintf("failed to list users: %v", err))
		return
	}

	found := false
	for _, user := range users {
		if user.InboundTag == inboundTag && user.SubID == subID {
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "USER_NOT_FOUND", fmt.Sprintf("user %s not found", subID))
		return
	}

	if err := h.singbox.DeleteUser(r.Context(), subID); err != nil {
		h.mapClientError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// extractInboundTag extracts the inbound tag from the path.
// Expected path format: /inbounds/{tag}/users or /inbounds/{tag}/users/{subId}
func extractInboundTag(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) >= 3 && parts[1] == "inbounds" {
		return parts[2]
	}
	return ""
}

// extractInboundTagAndSubID extracts the inbound tag and subID from the path.
// Expected path format: /inbounds/{tag}/users/{subId}
func extractInboundTagAndSubID(path string) (inboundTag, subID string) {
	parts := strings.Split(path, "/")
	if len(parts) >= 5 && parts[1] == "inbounds" && parts[3] == "users" {
		// ensure tag and subID are non-empty
		if parts[2] == "" || parts[4] == "" {
			return "", ""
		}
		return parts[2], parts[4]
	}
	return "", ""
}

func (h *UserHandler) inboundExists(r *http.Request, inboundTag string) bool {
	inbounds, err := h.singbox.GetInbounds(r.Context())
	if err != nil {
		return false
	}
	for _, inbound := range inbounds {
		if inbound.Tag == inboundTag {
			return true
		}
	}
	return false
}

func (h *UserHandler) mapClientError(w http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "already exists"):
		writeError(w, http.StatusConflict, "USER_EXISTS", message)
	case strings.Contains(message, "not found"):
		writeError(w, http.StatusNotFound, "USER_NOT_FOUND", message)
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", message)
	}
}
