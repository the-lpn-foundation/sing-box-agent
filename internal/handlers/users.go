package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/lenya/sing-box-agent/internal/models"
	syncpkg "github.com/lenya/sing-box-agent/internal/sync"
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

// NewUserHandler creates a new user handler.
func NewUserHandler() *UserHandler {
	return &UserHandler{}
}

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

// Mock user store for testing (will be replaced with sync engine integration)
var (
	mockInbounds = map[string]bool{
		"vless-reality": true,
		"hysteria2":     true,
	}
	mockUsers = make(map[string]map[string]models.User) // inboundTag -> subID -> User
)

func init() {
	// Initialize mock user store for testing
	mockUsers["vless-reality"] = make(map[string]models.User)
	mockUsers["hysteria2"] = make(map[string]models.User)
}

// ListUsers handles GET /inbounds/{tag}/users.
func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if h.singbox != nil {
		h.listUsersFromClient(w, r)
		return
	}

	// Extract inbound tag from path
	inboundTag := extractInboundTag(r.URL.Path)
	if inboundTag == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "inbound tag is required")
		return
	}

	// Check if inbound exists
	if !mockInbounds[inboundTag] {
		writeError(w, http.StatusNotFound, "INBOUND_NOT_FOUND", fmt.Sprintf("inbound %s not found", inboundTag))
		return
	}

	// Get users for this inbound
	users := mockUsers[inboundTag]
	userList := make([]models.User, 0, len(users))
	for _, user := range users {
		userList = append(userList, user)
	}

	writeSuccess(w, userList)
}

// GetUser handles GET /inbounds/{tag}/users/{subId}.
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	if h.singbox != nil {
		h.getUserFromClient(w, r)
		return
	}

	// Extract inbound tag and subID from path
	inboundTag, subID := extractInboundTagAndSubID(r.URL.Path)
	if inboundTag == "" || subID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "inbound tag and subID are required")
		return
	}

	// Check if inbound exists
	if !mockInbounds[inboundTag] {
		writeError(w, http.StatusNotFound, "INBOUND_NOT_FOUND", fmt.Sprintf("inbound %s not found", inboundTag))
		return
	}

	// Get user
	users, ok := mockUsers[inboundTag]
	if !ok {
		writeError(w, http.StatusNotFound, "USER_NOT_FOUND", fmt.Sprintf("user %s not found", subID))
		return
	}

	user, ok := users[subID]
	if !ok {
		writeError(w, http.StatusNotFound, "USER_NOT_FOUND", fmt.Sprintf("user %s not found", subID))
		return
	}

	writeSuccess(w, user)
}

// CreateUser handles POST /inbounds/{tag}/users.
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if h.singbox != nil {
		h.createUserWithClient(w, r)
		return
	}

	// Extract inbound tag from path
	inboundTag := extractInboundTag(r.URL.Path)
	if inboundTag == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "inbound tag is required")
		return
	}

	// Check if inbound exists
	if !mockInbounds[inboundTag] {
		writeError(w, http.StatusNotFound, "INBOUND_NOT_FOUND", fmt.Sprintf("inbound %s not found", inboundTag))
		return
	}

	// Parse request body
	var req UserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	// Check if user already exists (duplicate subID)
	users := mockUsers[inboundTag]
	if _, exists := users[req.SubID]; exists {
		writeError(w, http.StatusConflict, "USER_EXISTS", fmt.Sprintf("user %s already exists", req.SubID))
		return
	}

	// Create user
	user := req.ToUser(inboundTag)

	// Store user (this would trigger sync in real implementation)
	if mockUsers[inboundTag] == nil {
		mockUsers[inboundTag] = make(map[string]models.User)
	}
	mockUsers[inboundTag][user.SubID] = user

	// TODO: Trigger sync engine to apply changes
	// h.syncEngine.TriggerSync()

	writeCreated(w, user)
}

// UpdateUser handles PUT /inbounds/{tag}/users/{subId}.
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	if h.singbox != nil {
		h.updateUserWithClient(w, r)
		return
	}

	// Extract inbound tag and subID from path
	inboundTag, subID := extractInboundTagAndSubID(r.URL.Path)
	if inboundTag == "" || subID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "inbound tag and subID are required")
		return
	}

	// Check if inbound exists
	if !mockInbounds[inboundTag] {
		writeError(w, http.StatusNotFound, "INBOUND_NOT_FOUND", fmt.Sprintf("inbound %s not found", inboundTag))
		return
	}

	// Check if user exists
	users, ok := mockUsers[inboundTag]
	if !ok {
		writeError(w, http.StatusNotFound, "USER_NOT_FOUND", fmt.Sprintf("user %s not found", subID))
		return
	}

	_, ok = users[subID]
	if !ok {
		writeError(w, http.StatusNotFound, "USER_NOT_FOUND", fmt.Sprintf("user %s not found", subID))
		return
	}

	// Parse request body
	var req UserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	// Validate request (subID in body must match path)
	if req.SubID != "" && req.SubID != subID {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "subId in body must match subID in path")
		return
	}

	// Use subID from path
	req.SubID = subID

	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	// Update user
	user := req.ToUser(inboundTag)
	// Preserve UUID from existing user if not provided
	existingUser := users[subID]
	if req.UUID == "" {
		user.UUID = existingUser.UUID
	}

	// Store updated user (this would trigger sync in real implementation)
	mockUsers[inboundTag][user.SubID] = user

	// TODO: Trigger sync engine to apply changes
	// h.syncEngine.TriggerSync()

	writeSuccess(w, user)
}

// DeleteUser handles DELETE /inbounds/{tag}/users/{subId}.
func (h *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if h.singbox != nil {
		h.deleteUserWithClient(w, r)
		return
	}

	// Extract inbound tag and subID from path
	inboundTag, subID := extractInboundTagAndSubID(r.URL.Path)
	if inboundTag == "" || subID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "inbound tag and subID are required")
		return
	}

	// Check if inbound exists
	if !mockInbounds[inboundTag] {
		writeError(w, http.StatusNotFound, "INBOUND_NOT_FOUND", fmt.Sprintf("inbound %s not found", inboundTag))
		return
	}

	// Check if user exists
	users, ok := mockUsers[inboundTag]
	if !ok {
		writeError(w, http.StatusNotFound, "USER_NOT_FOUND", fmt.Sprintf("user %s not found", subID))
		return
	}

	_, ok = users[subID]
	if !ok {
		writeError(w, http.StatusNotFound, "USER_NOT_FOUND", fmt.Sprintf("user %s not found", subID))
		return
	}

	// Delete user (this would trigger sync in real implementation)
	delete(users, subID)

	// TODO: Trigger sync engine to apply changes
	// h.syncEngine.TriggerSync()

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

func (h *UserHandler) listUsersFromClient(w http.ResponseWriter, r *http.Request) {
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

func (h *UserHandler) getUserFromClient(w http.ResponseWriter, r *http.Request) {
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

func (h *UserHandler) createUserWithClient(w http.ResponseWriter, r *http.Request) {
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

func (h *UserHandler) updateUserWithClient(w http.ResponseWriter, r *http.Request) {
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

func (h *UserHandler) deleteUserWithClient(w http.ResponseWriter, r *http.Request) {
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
