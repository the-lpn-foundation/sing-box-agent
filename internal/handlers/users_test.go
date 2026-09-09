package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oglenyaboss/sing-box-agent/internal/models"
)

// TestUserRequestValidate tests the UserRequest.Validate method.
func TestUserRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     UserRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: UserRequest{
				SubID: "test-user-1",
				Email: "test@example.com",
			},
			wantErr: false,
		},
		{
			name:    "missing subId",
			req:     UserRequest{},
			wantErr: true,
		},
		{
			name: "invalid email format",
			req: UserRequest{
				SubID: "test-user-1",
				Email: "invalid-email",
			},
			wantErr: true,
		},
		{
			name: "valid email with @",
			req: UserRequest{
				SubID: "test-user-1",
				Email: "user@domain.com",
			},
			wantErr: false,
		},
		{
			name: "negative limitIp",
			req: UserRequest{
				SubID:   "test-user-1",
				LimitIP: intPtr(-1),
			},
			wantErr: true,
		},
		{
			name: "zero limitIp is valid",
			req: UserRequest{
				SubID:   "test-user-1",
				LimitIP: intPtr(0),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("UserRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestUserRequestToUser tests the UserRequest.ToUser method.
func TestUserRequestToUser(t *testing.T) {
	tests := []struct {
		name        string
		req         UserRequest
		inboundTag  string
		wantUUIDSet bool
	}{
		{
			name: "with UUID",
			req: UserRequest{
				SubID: "test-user-1",
				UUID:  "123e4567-e89b-12d3-a456-426614174000",
				Email: "test@example.com",
			},
			inboundTag:  "vless-reality",
			wantUUIDSet: true,
		},
		{
			name: "without UUID - should generate",
			req: UserRequest{
				SubID: "test-user-2",
				Email: "test2@example.com",
			},
			inboundTag:  "vless-reality",
			wantUUIDSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := tt.req.ToUser(tt.inboundTag)

			if user.SubID != tt.req.SubID {
				t.Errorf("ToUser() SubId = %v, want %v", user.SubID, tt.req.SubID)
			}

			if user.InboundTag != tt.inboundTag {
				t.Errorf("ToUser() InboundTag = %v, want %v", user.InboundTag, tt.inboundTag)
			}

			if tt.wantUUIDSet && user.UUID == "" {
				t.Errorf("ToUser() UUID should be set")
			}

			if tt.req.UUID != "" && user.UUID != tt.req.UUID {
				t.Errorf("ToUser() UUID = %v, want %v", user.UUID, tt.req.UUID)
			}
		})
	}
}

// fakeUserStore is a stateful in-memory syncpkg.SingBoxClient used to
// exercise the client-path user handlers without a live sing-box.
type fakeUserStore struct {
	inbounds []models.Inbound
	users    map[string]models.User // keyed by SubID
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{
		inbounds: []models.Inbound{
			{Tag: "vless-reality", Type: "vless"},
			{Tag: "hysteria2", Type: "hysteria2"},
		},
		users: make(map[string]models.User),
	}
}

func (f *fakeUserStore) GetInbounds(ctx context.Context) ([]models.Inbound, error) {
	return f.inbounds, nil
}

func (f *fakeUserStore) GetUsers(ctx context.Context) ([]models.User, error) {
	users := make([]models.User, 0, len(f.users))
	for _, user := range f.users {
		users = append(users, user)
	}
	return users, nil
}

func (f *fakeUserStore) CreateInbound(ctx context.Context, inbound models.Inbound) error {
	return nil
}

func (f *fakeUserStore) UpdateInbound(ctx context.Context, inbound models.Inbound) error {
	return nil
}

func (f *fakeUserStore) DeleteInbound(ctx context.Context, tag string) error {
	return nil
}

func (f *fakeUserStore) CreateUser(ctx context.Context, user models.User) error {
	if _, exists := f.users[user.SubID]; exists {
		return fmt.Errorf("user with subID %s already exists", user.SubID)
	}
	f.users[user.SubID] = user
	return nil
}

func (f *fakeUserStore) UpdateUser(ctx context.Context, user models.User) error {
	if _, exists := f.users[user.SubID]; !exists {
		return fmt.Errorf("user with subID %s not found", user.SubID)
	}
	f.users[user.SubID] = user
	return nil
}

func (f *fakeUserStore) DeleteUser(ctx context.Context, subID string) error {
	if _, exists := f.users[subID]; !exists {
		return fmt.Errorf("user with subID %s not found", subID)
	}
	delete(f.users, subID)
	return nil
}

// TestListUsers tests the ListUsers handler via a fake sing-box client.
func TestListUsers(t *testing.T) {
	store := newFakeUserStore()
	store.users["user1"] = models.User{
		SubID:      "user1",
		UUID:       "uuid1",
		InboundTag: "vless-reality",
		Enabled:    true,
		Email:      "user1@example.com",
	}
	store.users["user2"] = models.User{
		SubID:      "user2",
		UUID:       "uuid2",
		InboundTag: "vless-reality",
		Enabled:    true,
		Email:      "user2@example.com",
	}
	handler := NewUserHandlerWithClient(store)

	tests := []struct {
		name          string
		path          string
		wantStatus    int
		wantErrorCode string
		wantUserCount int
	}{
		{
			name:          "list users successfully",
			path:          "/inbounds/vless-reality/users",
			wantStatus:    http.StatusOK,
			wantUserCount: 2,
		},
		{
			name:          "empty user list",
			path:          "/inbounds/hysteria2/users",
			wantStatus:    http.StatusOK,
			wantUserCount: 0,
		},
		{
			name:          "inbound not found",
			path:          "/inbounds/non-existent/users",
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "INBOUND_NOT_FOUND",
		},
		{
			name:          "missing inbound tag",
			path:          "/inbounds//users",
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "INVALID_REQUEST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			handler.ListUsers(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("ListUsers() status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantErrorCode != "" {
				var resp ErrorResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.Error.Code != tt.wantErrorCode {
					t.Errorf("ListUsers() error code = %s, want %s", resp.Error.Code, tt.wantErrorCode)
				}
			} else if tt.wantUserCount >= 0 {
				var resp SuccessResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				users, ok := resp.Data.([]interface{})
				if !ok {
					t.Fatalf("Response data is not an array")
				}
				if len(users) != tt.wantUserCount {
					t.Errorf("ListUsers() user count = %d, want %d", len(users), tt.wantUserCount)
				}
			}
		})
	}
}

// TestGetUser tests the GetUser handler via a fake sing-box client.
func TestGetUser(t *testing.T) {
	store := newFakeUserStore()
	store.users["user1"] = models.User{
		SubID:      "user1",
		UUID:       "uuid1",
		InboundTag: "vless-reality",
		Enabled:    true,
		Email:      "user1@example.com",
	}
	handler := NewUserHandlerWithClient(store)

	tests := []struct {
		name          string
		path          string
		wantStatus    int
		wantErrorCode string
		wantSubID     string
	}{
		{
			name:       "get user successfully",
			path:       "/inbounds/vless-reality/users/user1",
			wantStatus: http.StatusOK,
			wantSubID:  "user1",
		},
		{
			name:          "user not found",
			path:          "/inbounds/vless-reality/users/non-existent",
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "USER_NOT_FOUND",
		},
		{
			name:          "inbound not found",
			path:          "/inbounds/non-existent/users/user1",
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "INBOUND_NOT_FOUND",
		},
		{
			name:          "missing parameters",
			path:          "/inbounds//users/",
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "INVALID_REQUEST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			handler.GetUser(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetUser() status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantErrorCode != "" {
				var resp ErrorResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.Error.Code != tt.wantErrorCode {
					t.Errorf("GetUser() error code = %s, want %s", resp.Error.Code, tt.wantErrorCode)
				}
			} else if tt.wantSubID != "" {
				var resp SuccessResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				userBytes, _ := json.Marshal(resp.Data)
				var user models.User
				if err := json.Unmarshal(userBytes, &user); err != nil {
					t.Fatalf("Failed to unmarshal user: %v", err)
				}
				if user.SubID != tt.wantSubID {
					t.Errorf("GetUser() subID = %s, want %s", user.SubID, tt.wantSubID)
				}
			}
		})
	}
}

// TestCreateUser tests the CreateUser handler via a fake sing-box client.
func TestCreateUser(t *testing.T) {
	store := newFakeUserStore()
	store.users["existing-user"] = models.User{
		SubID: "existing-user",
		UUID:  "uuid-existing",
	}
	handler := NewUserHandlerWithClient(store)

	tests := []struct {
		name          string
		path          string
		body          UserRequest
		wantStatus    int
		wantErrorCode string
		wantSubID     string
	}{
		{
			name: "create user successfully",
			path: "/inbounds/vless-reality/users",
			body: UserRequest{
				SubID: "new-user",
				Email: "newuser@example.com",
			},
			wantStatus: http.StatusCreated,
			wantSubID:  "new-user",
		},
		{
			name: "create user with UUID",
			path: "/inbounds/vless-reality/users",
			body: UserRequest{
				SubID: "user-with-uuid",
				UUID:  "123e4567-e89b-12d3-a456-426614174000",
				Email: "userwithuuid@example.com",
			},
			wantStatus: http.StatusCreated,
			wantSubID:  "user-with-uuid",
		},
		{
			name: "duplicate subId",
			path: "/inbounds/vless-reality/users",
			body: UserRequest{
				SubID: "existing-user",
				Email: "existing@example.com",
			},
			wantStatus:    http.StatusConflict,
			wantErrorCode: "USER_EXISTS",
		},
		{
			name:          "inbound not found",
			path:          "/inbounds/non-existent/users",
			body:          UserRequest{SubID: "user1"},
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "INBOUND_NOT_FOUND",
		},
		{
			name:          "invalid request body",
			path:          "/inbounds/vless-reality/users",
			body:          UserRequest{},
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "INVALID_REQUEST",
		},
		{
			name: "missing subId",
			path: "/inbounds/vless-reality/users",
			body: UserRequest{
				Email: "user@example.com",
			},
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "INVALID_REQUEST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.CreateUser(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("CreateUser() status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantErrorCode != "" {
				var resp ErrorResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.Error.Code != tt.wantErrorCode {
					t.Errorf("CreateUser() error code = %s, want %s", resp.Error.Code, tt.wantErrorCode)
				}
			} else if tt.wantSubID != "" {
				var resp SuccessResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				userBytes, _ := json.Marshal(resp.Data)
				var user models.User
				if err := json.Unmarshal(userBytes, &user); err != nil {
					t.Fatalf("Failed to unmarshal user: %v", err)
				}
				if user.SubID != tt.wantSubID {
					t.Errorf("CreateUser() subID = %s, want %s", user.SubID, tt.wantSubID)
				}
				if user.UUID == "" {
					t.Errorf("CreateUser() UUID should be set")
				}
			}
		})
	}
}

// TestUpdateUser tests the UpdateUser handler via a fake sing-box client.
func TestUpdateUser(t *testing.T) {
	store := newFakeUserStore()
	store.users["user1"] = models.User{
		SubID:      "user1",
		UUID:       "original-uuid",
		InboundTag: "vless-reality",
		Enabled:    true,
		Email:      "original@example.com",
	}
	handler := NewUserHandlerWithClient(store)

	tests := []struct {
		name          string
		path          string
		body          UserRequest
		wantStatus    int
		wantErrorCode string
	}{
		{
			name: "update user successfully",
			path: "/inbounds/vless-reality/users/user1",
			body: UserRequest{
				Email: "updated@example.com",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "update user with new UUID",
			path: "/inbounds/vless-reality/users/user1",
			body: UserRequest{
				SubID: "user1",
				UUID:  "new-uuid",
				Email: "updated@example.com",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "subId in body does not match path",
			path: "/inbounds/vless-reality/users/user1",
			body: UserRequest{
				SubID: "different-user",
				Email: "updated@example.com",
			},
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "INVALID_REQUEST",
		},
		{
			name:          "user not found",
			path:          "/inbounds/vless-reality/users/non-existent",
			body:          UserRequest{Email: "updated@example.com"},
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "USER_NOT_FOUND",
		},
		{
			name:          "inbound not found",
			path:          "/inbounds/non-existent/users/user1",
			body:          UserRequest{Email: "updated@example.com"},
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "INBOUND_NOT_FOUND",
		},
		{
			name:       "missing subID in body (should use path)",
			path:       "/inbounds/vless-reality/users/user1",
			body:       UserRequest{Email: "updated@example.com"},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPut, tt.path, bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.UpdateUser(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("UpdateUser() status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantErrorCode != "" {
				var resp ErrorResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.Error.Code != tt.wantErrorCode {
					t.Errorf("UpdateUser() error code = %s, want %s", resp.Error.Code, tt.wantErrorCode)
				}
			} else if tt.wantStatus == http.StatusOK {
				var resp SuccessResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				userBytes, _ := json.Marshal(resp.Data)
				var user models.User
				if err := json.Unmarshal(userBytes, &user); err != nil {
					t.Fatalf("Failed to unmarshal user: %v", err)
				}

				// Verify the update was applied
				if tt.body.Email != "" && user.Email != tt.body.Email {
					t.Errorf("UpdateUser() email = %s, want %s", user.Email, tt.body.Email)
				}
			}
		})
	}
}

// TestDeleteUser tests the DeleteUser handler via a fake sing-box client.
func TestDeleteUser(t *testing.T) {
	store := newFakeUserStore()
	handler := NewUserHandlerWithClient(store)

	tests := []struct {
		name          string
		path          string
		wantStatus    int
		wantErrorCode string
	}{
		{
			name:       "delete user successfully",
			path:       "/inbounds/vless-reality/users/user1",
			wantStatus: http.StatusNoContent,
		},
		{
			name:          "user not found",
			path:          "/inbounds/vless-reality/users/non-existent",
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "USER_NOT_FOUND",
		},
		{
			name:          "inbound not found",
			path:          "/inbounds/non-existent/users/user1",
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "INBOUND_NOT_FOUND",
		},
		{
			name:          "missing parameters",
			path:          "/inbounds//users/",
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "INVALID_REQUEST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-seed the user before each successful delete
			store.users["user1"] = models.User{
				SubID:      "user1",
				UUID:       "uuid1",
				InboundTag: "vless-reality",
				Enabled:    true,
				Email:      "user1@example.com",
			}

			req := httptest.NewRequest(http.MethodDelete, tt.path, nil)
			w := httptest.NewRecorder()

			handler.DeleteUser(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("DeleteUser() status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantErrorCode != "" {
				var resp ErrorResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.Error.Code != tt.wantErrorCode {
					t.Errorf("DeleteUser() error code = %s, want %s", resp.Error.Code, tt.wantErrorCode)
				}
			} else if tt.name == "delete user successfully" {
				// Verify user was deleted
				if _, exists := store.users["user1"]; exists {
					t.Errorf("DeleteUser() user still exists after deletion")
				}
			}
		})
	}
}

// TestUserHandler_NilClientGuard verifies that every user handler responds
// with 503 instead of panicking when no sing-box client is configured.
func TestUserHandler_NilClientGuard(t *testing.T) {
	handler := NewUserHandlerWithClient(nil)

	tests := []struct {
		name   string
		method string
		path   string
		call   func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:   "ListUsers",
			method: http.MethodGet,
			path:   "/inbounds/vless-reality/users",
			call:   handler.ListUsers,
		},
		{
			name:   "GetUser",
			method: http.MethodGet,
			path:   "/inbounds/vless-reality/users/user1",
			call:   handler.GetUser,
		},
		{
			name:   "CreateUser",
			method: http.MethodPost,
			path:   "/inbounds/vless-reality/users",
			call:   handler.CreateUser,
		},
		{
			name:   "UpdateUser",
			method: http.MethodPut,
			path:   "/inbounds/vless-reality/users/user1",
			call:   handler.UpdateUser,
		},
		{
			name:   "DeleteUser",
			method: http.MethodDelete,
			path:   "/inbounds/vless-reality/users/user1",
			call:   handler.DeleteUser,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			tt.call(w, req)

			if w.Code != http.StatusServiceUnavailable {
				t.Errorf("%s with nil client: status = %d, want %d", tt.name, w.Code, http.StatusServiceUnavailable)
			}
			var resp ErrorResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}
			if resp.Error.Code != "INTERNAL_ERROR" {
				t.Errorf("%s with nil client: error code = %s, want INTERNAL_ERROR", tt.name, resp.Error.Code)
			}
		})
	}
}

// TestExtractInboundTag tests the extractInboundTag helper function.
func TestExtractInboundTag(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantTag string
	}{
		{
			name:    "valid path with users",
			path:    "/inbounds/vless-reality/users",
			wantTag: "vless-reality",
		},
		{
			name:    "valid path with subId",
			path:    "/inbounds/vless-reality/users/user1",
			wantTag: "vless-reality",
		},
		{
			name:    "invalid path - missing tag",
			path:    "/inbounds//users",
			wantTag: "",
		},
		{
			name:    "invalid path - wrong prefix",
			path:    "/other/vless-reality/users",
			wantTag: "",
		},
		{
			name:    "empty path",
			path:    "",
			wantTag: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractInboundTag(tt.path)
			if got != tt.wantTag {
				t.Errorf("extractInboundTag() = %v, want %v", got, tt.wantTag)
			}
		})
	}
}

// TestExtractInboundTagAndSubId tests the extractInboundTagAndSubID helper function.
func TestExtractInboundTagAndSubId(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantTag   string
		wantSubID string
	}{
		{
			name:      "valid path",
			path:      "/inbounds/vless-reality/users/user1",
			wantTag:   "vless-reality",
			wantSubID: "user1",
		},
		{
			name:      "missing subId",
			path:      "/inbounds/vless-reality/users",
			wantTag:   "",
			wantSubID: "",
		},
		{
			name:      "missing tag",
			path:      "/inbounds//users/user1",
			wantTag:   "",
			wantSubID: "",
		},
		{
			name:      "wrong prefix",
			path:      "/other/vless-reality/users/user1",
			wantTag:   "",
			wantSubID: "",
		},
		{
			name:      "empty path",
			path:      "",
			wantTag:   "",
			wantSubID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTag, gotSubID := extractInboundTagAndSubID(tt.path)
			if gotTag != tt.wantTag {
				t.Errorf("extractInboundTagAndSubID() tag = %v, want %v", gotTag, tt.wantTag)
			}
			if gotSubID != tt.wantSubID {
				t.Errorf("extractInboundTagAndSubID() subID = %v, want %v", gotSubID, tt.wantSubID)
			}
		})
	}
}

// Helper function to create a pointer to an int.
func intPtr(i int) *int {
	return &i
}

// TestNewUserHandlerWithClient tests NewUserHandlerWithClient.
func TestNewUserHandlerWithClient(t *testing.T) {
	mockClient := &mockUserSingBoxClient{}
	handler := NewUserHandlerWithClient(mockClient)

	if handler == nil {
		t.Fatal("NewUserHandlerWithClient() returned nil")
	}

	if handler.singbox != mockClient {
		t.Error("NewUserHandlerWithClient() did not set singbox client")
	}
}

// TestListUsersWithClient tests ListUsers.
func TestListUsersWithClient(t *testing.T) {
	mockClient := &mockUserSingBoxClient{
		users: []models.User{
			{SubID: "user1", InboundTag: "vless-reality", Enabled: true},
			{SubID: "user2", InboundTag: "vless-reality", Enabled: true},
			{SubID: "user3", InboundTag: "hysteria2", Enabled: true},
		},
		inbounds: []models.Inbound{
			{Tag: "vless-reality", Type: "vless"},
			{Tag: "hysteria2", Type: "hysteria2"},
		},
	}
	handler := NewUserHandlerWithClient(mockClient)

	tests := []struct {
		name          string
		path          string
		wantStatus    int
		wantErrorCode string
		wantUserCount int
	}{
		{
			name:          "list users successfully",
			path:          "/inbounds/vless-reality/users",
			wantStatus:    http.StatusOK,
			wantUserCount: 2,
		},
		{
			name:          "empty user list",
			path:          "/inbounds/hysteria2/users",
			wantStatus:    http.StatusOK,
			wantUserCount: 1,
		},
		{
			name:          "inbound not found",
			path:          "/inbounds/non-existent/users",
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "INBOUND_NOT_FOUND",
		},
		{
			name:          "missing inbound tag",
			path:          "/inbounds//users",
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "INVALID_REQUEST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "client error" {
				mockClient.failOnGetUsers = true
				mockClient.failOnGetInbounds = false
			} else {
				mockClient.failOnGetUsers = false
				mockClient.failOnGetInbounds = false
			}

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			handler.ListUsers(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("ListUsers() status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantErrorCode != "" {
				var resp ErrorResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.Error.Code != tt.wantErrorCode {
					t.Errorf("ListUsers() error code = %s, want %s", resp.Error.Code, tt.wantErrorCode)
				}
			} else if tt.wantUserCount >= 0 {
				var resp SuccessResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				users, ok := resp.Data.([]interface{})
				if !ok {
					t.Fatalf("Response data is not an array")
				}
				if len(users) != tt.wantUserCount {
					t.Errorf("ListUsers() user count = %d, want %d", len(users), tt.wantUserCount)
				}
			}
		})
	}
}

// TestGetUserWithClient tests GetUser.
func TestGetUserWithClient(t *testing.T) {
	mockClient := &mockUserSingBoxClient{
		users: []models.User{
			{SubID: "user1", UUID: "uuid1", InboundTag: "vless-reality", Enabled: true, Email: "user1@example.com"},
			{SubID: "user2", UUID: "uuid2", InboundTag: "hysteria2", Enabled: true},
		},
		inbounds: []models.Inbound{
			{Tag: "vless-reality", Type: "vless"},
			{Tag: "hysteria2", Type: "hysteria2"},
		},
	}
	handler := NewUserHandlerWithClient(mockClient)

	tests := []struct {
		name          string
		path          string
		wantStatus    int
		wantErrorCode string
		wantSubID     string
	}{
		{
			name:       "get user successfully",
			path:       "/inbounds/vless-reality/users/user1",
			wantStatus: http.StatusOK,
			wantSubID:  "user1",
		},
		{
			name:          "user not found",
			path:          "/inbounds/vless-reality/users/non-existent",
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "USER_NOT_FOUND",
		},
		{
			name:          "inbound not found",
			path:          "/inbounds/non-existent/users/user1",
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "INBOUND_NOT_FOUND",
		},
		{
			name:          "missing parameters",
			path:          "/inbounds//users/",
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "INVALID_REQUEST",
		},
		{
			name:          "client error",
			path:          "/inbounds/vless-reality/users/user1",
			wantStatus:    http.StatusInternalServerError,
			wantErrorCode: "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "client error" {
				mockClient.failOnGetUsers = true
			} else {
				mockClient.failOnGetUsers = false
			}

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			handler.GetUser(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetUser() status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantErrorCode != "" {
				var resp ErrorResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.Error.Code != tt.wantErrorCode {
					t.Errorf("GetUser() error code = %s, want %s", resp.Error.Code, tt.wantErrorCode)
				}
			} else if tt.wantSubID != "" {
				var resp SuccessResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				userBytes, _ := json.Marshal(resp.Data)
				var user models.User
				if err := json.Unmarshal(userBytes, &user); err != nil {
					t.Fatalf("Failed to unmarshal user: %v", err)
				}
				if user.SubID != tt.wantSubID {
					t.Errorf("GetUser() subID = %s, want %s", user.SubID, tt.wantSubID)
				}
			}
		})
	}
}

// TestCreateUserWithClient tests CreateUser.
func TestCreateUserWithClient(t *testing.T) {
	mockClient := &mockUserSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "vless-reality", Type: "vless"},
			{Tag: "hysteria2", Type: "hysteria2"},
		},
	}
	handler := NewUserHandlerWithClient(mockClient)

	tests := []struct {
		name          string
		path          string
		body          UserRequest
		wantStatus    int
		wantErrorCode string
		wantSubID     string
	}{
		{
			name: "create user successfully",
			path: "/inbounds/vless-reality/users",
			body: UserRequest{
				SubID: "new-user",
				Email: "newuser@example.com",
			},
			wantStatus: http.StatusCreated,
			wantSubID:  "new-user",
		},
		{
			name: "create user with UUID",
			path: "/inbounds/vless-reality/users",
			body: UserRequest{
				SubID: "user-with-uuid",
				UUID:  "123e4567-e89b-12d3-a456-426614174000",
				Email: "userwithuuid@example.com",
			},
			wantStatus: http.StatusCreated,
			wantSubID:  "user-with-uuid",
		},
		{
			name:          "inbound not found",
			path:          "/inbounds/non-existent/users",
			body:          UserRequest{SubID: "user1"},
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "INBOUND_NOT_FOUND",
		},
		{
			name:          "invalid request body",
			path:          "/inbounds/vless-reality/users",
			body:          UserRequest{},
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "INVALID_REQUEST",
		},
		{
			name: "missing subId",
			path: "/inbounds/vless-reality/users",
			body: UserRequest{
				Email: "user@example.com",
			},
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "INVALID_REQUEST",
		},
		{
			name:          "user already exists",
			path:          "/inbounds/vless-reality/users",
			body:          UserRequest{SubID: "existing-user"},
			wantStatus:    http.StatusConflict,
			wantErrorCode: "USER_EXISTS",
		},
		{
			name:          "client error",
			path:          "/inbounds/vless-reality/users",
			body:          UserRequest{SubID: "error-user"},
			wantStatus:    http.StatusInternalServerError,
			wantErrorCode: "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.name {
			case "user already exists":
				mockClient.failOnCreateExists = true
				mockClient.failOnCreate = false
			case "client error":
				mockClient.failOnCreate = true
				mockClient.failOnCreateExists = false
			default:
				mockClient.failOnCreate = false
				mockClient.failOnCreateExists = false
			}

			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.CreateUser(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("CreateUser() status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantErrorCode != "" {
				var resp ErrorResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.Error.Code != tt.wantErrorCode {
					t.Errorf("CreateUser() error code = %s, want %s", resp.Error.Code, tt.wantErrorCode)
				}
			} else if tt.wantSubID != "" {
				var resp SuccessResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				userBytes, _ := json.Marshal(resp.Data)
				var user models.User
				if err := json.Unmarshal(userBytes, &user); err != nil {
					t.Fatalf("Failed to unmarshal user: %v", err)
				}
				if user.SubID != tt.wantSubID {
					t.Errorf("CreateUser() subID = %s, want %s", user.SubID, tt.wantSubID)
				}
				if user.UUID == "" {
					t.Errorf("CreateUser() UUID should be set")
				}
			}
		})
	}
}

// TestUpdateUserWithClient tests UpdateUser.
func TestUpdateUserWithClient(t *testing.T) {
	mockClient := &mockUserSingBoxClient{
		users: []models.User{
			{SubID: "user1", UUID: "original-uuid", InboundTag: "vless-reality", Enabled: true, Email: "original@example.com"},
		},
		inbounds: []models.Inbound{
			{Tag: "vless-reality", Type: "vless"},
			{Tag: "hysteria2", Type: "hysteria2"},
		},
	}
	handler := NewUserHandlerWithClient(mockClient)

	tests := []struct {
		name          string
		path          string
		body          UserRequest
		wantStatus    int
		wantErrorCode string
	}{
		{
			name: "update user successfully",
			path: "/inbounds/vless-reality/users/user1",
			body: UserRequest{
				Email: "updated@example.com",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "update user with new UUID",
			path: "/inbounds/vless-reality/users/user1",
			body: UserRequest{
				SubID: "user1",
				UUID:  "new-uuid",
				Email: "updated@example.com",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "subId in body does not match path",
			path: "/inbounds/vless-reality/users/user1",
			body: UserRequest{
				SubID: "different-user",
				Email: "updated@example.com",
			},
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "INVALID_REQUEST",
		},
		{
			name:          "inbound not found",
			path:          "/inbounds/non-existent/users/user1",
			body:          UserRequest{Email: "updated@example.com"},
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "INBOUND_NOT_FOUND",
		},
		{
			name:          "missing subID in body (should use path)",
			path:          "/inbounds/vless-reality/users/user1",
			body:          UserRequest{Email: "updated@example.com"},
			wantStatus:    http.StatusOK,
			wantErrorCode: "",
		},
		{
			name:          "user not found",
			path:          "/inbounds/vless-reality/users/non-existent",
			body:          UserRequest{Email: "updated@example.com"},
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "USER_NOT_FOUND",
		},
		{
			name:          "client error",
			path:          "/inbounds/vless-reality/users/user1",
			body:          UserRequest{Email: "updated@example.com"},
			wantStatus:    http.StatusInternalServerError,
			wantErrorCode: "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.name {
			case "user not found":
				mockClient.failOnUpdateNotFound = true
				mockClient.failOnUpdate = false
			case "client error":
				mockClient.failOnUpdate = true
				mockClient.failOnUpdateNotFound = false
			default:
				mockClient.failOnUpdate = false
				mockClient.failOnUpdateNotFound = false
			}

			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPut, tt.path, bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.UpdateUser(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("UpdateUser() status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantErrorCode != "" {
				var resp ErrorResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.Error.Code != tt.wantErrorCode {
					t.Errorf("UpdateUser() error code = %s, want %s", resp.Error.Code, tt.wantErrorCode)
				}
			} else if tt.wantStatus == http.StatusOK {
				var resp SuccessResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				userBytes, _ := json.Marshal(resp.Data)
				var user models.User
				if err := json.Unmarshal(userBytes, &user); err != nil {
					t.Fatalf("Failed to unmarshal user: %v", err)
				}

				if tt.body.Email != "" && user.Email != tt.body.Email {
					t.Errorf("UpdateUser() email = %s, want %s", user.Email, tt.body.Email)
				}
			}
		})
	}
}

// TestDeleteUserWithClient tests DeleteUser.
func TestDeleteUserWithClient(t *testing.T) {
	mockClient := &mockUserSingBoxClient{
		users: []models.User{
			{SubID: "user1", UUID: "uuid1", InboundTag: "vless-reality", Enabled: true, Email: "user1@example.com"},
		},
		inbounds: []models.Inbound{
			{Tag: "vless-reality", Type: "vless"},
			{Tag: "hysteria2", Type: "hysteria2"},
		},
	}
	handler := NewUserHandlerWithClient(mockClient)

	tests := []struct {
		name          string
		path          string
		wantStatus    int
		wantErrorCode string
	}{
		{
			name:       "delete user successfully",
			path:       "/inbounds/vless-reality/users/user1",
			wantStatus: http.StatusNoContent,
		},
		{
			name:          "user not found",
			path:          "/inbounds/vless-reality/users/non-existent",
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "USER_NOT_FOUND",
		},
		{
			name:          "inbound not found",
			path:          "/inbounds/non-existent/users/user1",
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "INBOUND_NOT_FOUND",
		},
		{
			name:          "missing parameters",
			path:          "/inbounds//users/",
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "INVALID_REQUEST",
		},
		{
			name:          "client error on get users",
			path:          "/inbounds/vless-reality/users/user1",
			wantStatus:    http.StatusInternalServerError,
			wantErrorCode: "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.name {
			case "user not found":
				mockClient.failOnDeleteNotFound = true
				mockClient.failOnDelete = false
				mockClient.failOnGetUsers = false
			case "client error on get users":
				mockClient.failOnGetUsers = true
				mockClient.failOnDelete = false
				mockClient.failOnDeleteNotFound = false
			default:
				mockClient.failOnDelete = false
				mockClient.failOnDeleteNotFound = false
				mockClient.failOnGetUsers = false
			}

			req := httptest.NewRequest(http.MethodDelete, tt.path, nil)
			w := httptest.NewRecorder()

			handler.DeleteUser(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("DeleteUser() status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantErrorCode != "" {
				var resp ErrorResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.Error.Code != tt.wantErrorCode {
					t.Errorf("DeleteUser() error code = %s, want %s", resp.Error.Code, tt.wantErrorCode)
				}
			}
		})
	}
}

// TestInboundExists tests inboundExists.
func TestInboundExists(t *testing.T) {
	mockClient := &mockUserSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "vless-reality", Type: "vless"},
			{Tag: "hysteria2", Type: "hysteria2"},
		},
	}
	handler := NewUserHandlerWithClient(mockClient)

	tests := []struct {
		name       string
		inboundTag string
		wantExists bool
	}{
		{
			name:       "inbound exists",
			inboundTag: "vless-reality",
			wantExists: true,
		},
		{
			name:       "inbound does not exist",
			inboundTag: "non-existent",
			wantExists: false,
		},
		{
			name:       "client error",
			inboundTag: "error",
			wantExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "client error" {
				mockClient.failOnGetInbounds = true
			} else {
				mockClient.failOnGetInbounds = false
			}

			req := httptest.NewRequest(http.MethodGet, "/inbounds/"+tt.inboundTag+"/users", nil)
			exists := handler.inboundExists(req, tt.inboundTag)

			if exists != tt.wantExists {
				t.Errorf("inboundExists() = %v, want %v", exists, tt.wantExists)
			}
		})
	}
}

// TestMapClientError tests mapClientError.
func TestMapClientError(t *testing.T) {
	mockClient := &mockUserSingBoxClient{}
	handler := NewUserHandlerWithClient(mockClient)

	tests := []struct {
		name          string
		err           error
		wantStatus    int
		wantErrorCode string
	}{
		{
			name:          "already exists error",
			err:           errors.New("user already exists"),
			wantStatus:    http.StatusConflict,
			wantErrorCode: "USER_EXISTS",
		},
		{
			name:          "not found error",
			err:           errors.New("user not found"),
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "USER_NOT_FOUND",
		},
		{
			name:          "generic error",
			err:           errors.New("some other error"),
			wantStatus:    http.StatusInternalServerError,
			wantErrorCode: "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			handler.mapClientError(w, tt.err)

			if w.Code != tt.wantStatus {
				t.Errorf("mapClientError() status = %d, want %d", w.Code, tt.wantStatus)
			}

			var resp ErrorResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}
			if resp.Error.Code != tt.wantErrorCode {
				t.Errorf("mapClientError() error code = %s, want %s", resp.Error.Code, tt.wantErrorCode)
			}
		})
	}
}

// TestUserRequestToUser_PartialCoverage tests additional ToUser cases.
func TestUserRequestToUser_PartialCoverage(t *testing.T) {
	tests := []struct {
		name       string
		req        UserRequest
		inboundTag string
		wantUser   models.User
	}{
		{
			name: "with all fields",
			req: UserRequest{
				SubID:         "test-user",
				UUID:          "123e4567-e89b-12d3-a456-426614174000",
				Email:         "test@example.com",
				Enabled:       boolPtr(false),
				Flow:          "xtls-rprx-vision",
				LimitIP:       intPtr(5),
				UploadLimit:   1000000,
				DownloadLimit: 2000000,
			},
			inboundTag: "vless-reality",
			wantUser: models.User{
				SubID:         "test-user",
				UUID:          "123e4567-e89b-12d3-a456-426614174000",
				InboundTag:    "vless-reality",
				Email:         "test@example.com",
				Enabled:       false,
				Flow:          "xtls-rprx-vision",
				LimitIP:       5,
				UploadLimit:   1000000,
				DownloadLimit: 2000000,
			},
		},
		{
			name: "with nil enabled (should default to true)",
			req: UserRequest{
				SubID: "test-user",
				Email: "test@example.com",
			},
			inboundTag: "vless-reality",
			wantUser: models.User{
				SubID:      "test-user",
				InboundTag: "vless-reality",
				Email:      "test@example.com",
				Enabled:    true,
			},
		},
		{
			name: "with nil limitIP (should not set)",
			req: UserRequest{
				SubID: "test-user",
				Email: "test@example.com",
			},
			inboundTag: "vless-reality",
			wantUser: models.User{
				SubID:      "test-user",
				InboundTag: "vless-reality",
				Email:      "test@example.com",
				Enabled:    true,
				LimitIP:    0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := tt.req.ToUser(tt.inboundTag)

			if user.SubID != tt.wantUser.SubID {
				t.Errorf("ToUser() SubID = %v, want %v", user.SubID, tt.wantUser.SubID)
			}
			if tt.wantUser.UUID != "" {
				if user.UUID != tt.wantUser.UUID {
					t.Errorf("ToUser() UUID = %v, want %v", user.UUID, tt.wantUser.UUID)
				}
			} else {
				// If no UUID expected, check that one was generated
				if user.UUID == "" {
					t.Errorf("ToUser() UUID should be generated when not provided")
				}
			}
			if user.InboundTag != tt.wantUser.InboundTag {
				t.Errorf("ToUser() InboundTag = %v, want %v", user.InboundTag, tt.wantUser.InboundTag)
			}
			if user.Email != tt.wantUser.Email {
				t.Errorf("ToUser() Email = %v, want %v", user.Email, tt.wantUser.Email)
			}
			if user.Enabled != tt.wantUser.Enabled {
				t.Errorf("ToUser() Enabled = %v, want %v", user.Enabled, tt.wantUser.Enabled)
			}
			if user.Flow != tt.wantUser.Flow {
				t.Errorf("ToUser() Flow = %v, want %v", user.Flow, tt.wantUser.Flow)
			}
			if user.LimitIP != tt.wantUser.LimitIP {
				t.Errorf("ToUser() LimitIP = %v, want %v", user.LimitIP, tt.wantUser.LimitIP)
			}
			if user.UploadLimit != tt.wantUser.UploadLimit {
				t.Errorf("ToUser() UploadLimit = %v, want %v", user.UploadLimit, tt.wantUser.UploadLimit)
			}
			if user.DownloadLimit != tt.wantUser.DownloadLimit {
				t.Errorf("ToUser() DownloadLimit = %v, want %v", user.DownloadLimit, tt.wantUser.DownloadLimit)
			}
		})
	}
}

// Helper function to create a pointer to a bool.
func boolPtr(b bool) *bool {
	return &b
}

// mockUserSingBoxClient is a mock implementation of SingBoxClient for testing.
type mockUserSingBoxClient struct {
	users                []models.User
	inbounds             []models.Inbound
	failOnGetUsers       bool
	failOnGetInbounds    bool
	failOnCreate         bool
	failOnCreateExists   bool
	failOnUpdate         bool
	failOnUpdateNotFound bool
	failOnDelete         bool
	failOnDeleteNotFound bool
}

func (m *mockUserSingBoxClient) GetInbounds(ctx context.Context) ([]models.Inbound, error) {
	if m.failOnGetInbounds {
		return nil, errors.New("get inbounds error")
	}
	return m.inbounds, nil
}

func (m *mockUserSingBoxClient) GetUsers(ctx context.Context) ([]models.User, error) {
	if m.failOnGetUsers {
		return nil, errors.New("get users error")
	}
	return m.users, nil
}

func (m *mockUserSingBoxClient) CreateInbound(ctx context.Context, inbound models.Inbound) error {
	return nil
}

func (m *mockUserSingBoxClient) UpdateInbound(ctx context.Context, inbound models.Inbound) error {
	return nil
}

func (m *mockUserSingBoxClient) DeleteInbound(ctx context.Context, tag string) error {
	return nil
}

func (m *mockUserSingBoxClient) CreateUser(ctx context.Context, user models.User) error {
	if m.failOnCreate {
		return errors.New("create user error")
	}
	if m.failOnCreateExists {
		return errors.New("user already exists")
	}
	m.users = append(m.users, user)
	return nil
}

func (m *mockUserSingBoxClient) UpdateUser(ctx context.Context, user models.User) error {
	if m.failOnUpdate {
		return errors.New("update user error")
	}
	if m.failOnUpdateNotFound {
		return errors.New("user not found")
	}
	for i, u := range m.users {
		if u.SubID == user.SubID {
			m.users[i] = user
			return nil
		}
	}
	return errors.New("user not found")
}

func (m *mockUserSingBoxClient) DeleteUser(ctx context.Context, subID string) error {
	if m.failOnDelete {
		return errors.New("delete user error")
	}
	if m.failOnDeleteNotFound {
		return errors.New("user not found")
	}
	for i, u := range m.users {
		if u.SubID == subID {
			m.users = append(m.users[:i], m.users[i+1:]...)
			return nil
		}
	}
	return errors.New("user not found")
}
