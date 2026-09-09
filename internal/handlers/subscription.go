package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/the-lpn-foundation/sing-box-agent/internal/models"
	syncpkg "github.com/the-lpn-foundation/sing-box-agent/internal/sync"
)

const (
	SubscriptionFormatV2Ray   = "v2ray"
	SubscriptionFormatClash   = "clash"
	SubscriptionFormatSingBox = "sing-box"

	CodeSubscriptionNotFound = "USER_NOT_FOUND"

	// maxSubscriptionCacheEntries bounds the in-memory subscription cache so
	// that unbounded unique subIDs cannot grow it forever. Records beyond the
	// cap are simply not cached; GetSubscription regenerates them from live
	// sing-box state.
	maxSubscriptionCacheEntries = 1000
)

type SubscriptionRequest struct {
	SubID      string `json:"subId"`
	InboundTag string `json:"inboundTag,omitempty"`
	Format     string `json:"format,omitempty"`
	Server     string `json:"server,omitempty"`
	Port       int    `json:"port,omitempty"`
}

type SubscriptionRecord struct {
	SubID      string `json:"subId"`
	InboundTag string `json:"inboundTag,omitempty"`
	Format     string `json:"format"`
	Config     string `json:"config"`
}

type SubscriptionHandler struct {
	mu      sync.RWMutex
	store   map[string]SubscriptionRecord
	logger  *slog.Logger
	singbox syncpkg.SingBoxClient
}

func NewSubscriptionHandler(logger *slog.Logger, singbox ...syncpkg.SingBoxClient) *SubscriptionHandler {
	var client syncpkg.SingBoxClient
	if len(singbox) > 0 {
		client = singbox[0]
	}

	return &SubscriptionHandler{
		store:   make(map[string]SubscriptionRecord),
		logger:  logger,
		singbox: client,
	}
}

func (h *SubscriptionHandler) GenerateSubscription(w http.ResponseWriter, r *http.Request) {
	var req SubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	format := normalizeSubscriptionFormat(req.Format)
	if req.SubID == "" {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, "subId is required")
		return
	}

	if format == SubscriptionFormatV2Ray || format == SubscriptionFormatClash {
		if req.Server == "" {
			h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, "server is required for v2ray/clash format")
			return
		}
		if req.Port < 1 || req.Port > 65535 {
			h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, "port must be between 1 and 65535")
			return
		}

		config, err := buildLegacySubscriptionConfig(req, format)
		if err != nil {
			h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
			return
		}

		record := SubscriptionRecord{SubID: req.SubID, InboundTag: req.InboundTag, Format: format, Config: config}
		h.cacheRecord(record)
		h.sendSuccess(w, http.StatusOK, record)
		return
	}

	if h.singbox == nil {
		if req.Server == "" {
			h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, "server is required")
			return
		}
		if req.Port < 1 || req.Port > 65535 {
			h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, "port must be between 1 and 65535")
			return
		}

		config, err := buildLegacySubscriptionConfig(req, format)
		if err != nil {
			h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
			return
		}

		record := SubscriptionRecord{SubID: req.SubID, InboundTag: req.InboundTag, Format: format, Config: config}
		h.cacheRecord(record)
		h.sendSuccess(w, http.StatusOK, record)
		return
	}

	targetInbound, targetUser, allInbounds, err := h.resolveTarget(r.Context(), req.SubID, req.InboundTag)
	if err != nil {
		h.sendError(w, http.StatusNotFound, CodeSubscriptionNotFound, err.Error())
		return
	}

	server, port, err := resolveServerPort(req, targetInbound, r.Host)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}

	config, err := buildSingBoxSubscriptionConfig(format, targetInbound, targetUser, allInbounds, server, port)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}

	record := SubscriptionRecord{
		SubID:      req.SubID,
		InboundTag: targetInbound.Tag,
		Format:     format,
		Config:     config,
	}

	h.cacheRecord(record)

	h.sendSuccess(w, http.StatusOK, record)
}

// cacheRecord stores a subscription record in the in-memory cache unless the
// cache is at capacity, in which case the record is skipped (the response to
// the client is unaffected and GetSubscription regenerates on demand).
func (h *SubscriptionHandler) cacheRecord(record SubscriptionRecord) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.store) >= maxSubscriptionCacheEntries {
		return
	}
	h.store[record.SubID] = record
}

func (h *SubscriptionHandler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	subID := strings.TrimPrefix(r.URL.Path, "/subscription/")
	if subID == "" || subID == r.URL.Path {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, "subId is required")
		return
	}

	h.mu.RLock()
	record, ok := h.store[subID]
	h.mu.RUnlock()
	if ok {
		h.sendSuccess(w, http.StatusOK, record)
		return
	}

	if h.singbox == nil {
		h.sendError(w, http.StatusNotFound, CodeSubscriptionNotFound, fmt.Sprintf("subscription for %s not found", subID))
		return
	}

	targetInbound, targetUser, allInbounds, err := h.resolveTarget(r.Context(), subID, "")
	if err != nil {
		h.sendError(w, http.StatusNotFound, CodeSubscriptionNotFound, fmt.Sprintf("subscription for %s not found", subID))
		return
	}

	server, port, err := resolveServerPort(SubscriptionRequest{SubID: subID}, targetInbound, r.Host)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}

	config, err := buildSingBoxSubscriptionConfig(SubscriptionFormatSingBox, targetInbound, targetUser, allInbounds, server, port)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}

	record = SubscriptionRecord{
		SubID:      subID,
		InboundTag: targetInbound.Tag,
		Format:     SubscriptionFormatSingBox,
		Config:     config,
	}

	h.cacheRecord(record)

	h.sendSuccess(w, http.StatusOK, record)
}

func (h *SubscriptionHandler) resolveTarget(ctx context.Context, subID, inboundTag string) (models.Inbound, models.User, []models.Inbound, error) {
	inbounds, err := h.singbox.GetInbounds(ctx)
	if err != nil {
		return models.Inbound{}, models.User{}, nil, fmt.Errorf("failed to load inbounds: %w", err)
	}
	users, err := h.singbox.GetUsers(ctx)
	if err != nil {
		return models.Inbound{}, models.User{}, nil, fmt.Errorf("failed to load users: %w", err)
	}

	for _, user := range users {
		if user.SubID != subID {
			continue
		}
		if inboundTag != "" && user.InboundTag != inboundTag {
			continue
		}

		for _, inbound := range inbounds {
			if inbound.Tag == user.InboundTag {
				return inbound, user, inbounds, nil
			}
		}
	}

	return models.Inbound{}, models.User{}, inbounds, fmt.Errorf("subscription for %s not found", subID)
}

func normalizeSubscriptionFormat(format string) string {
	f := strings.ToLower(strings.TrimSpace(format))
	if f == "" || f == "singbox" {
		return SubscriptionFormatSingBox
	}
	return f
}

func buildLegacySubscriptionConfig(req SubscriptionRequest, format string) (string, error) {
	port := strconv.Itoa(req.Port)

	switch format {
	case SubscriptionFormatV2Ray:
		return fmt.Sprintf("vless://%s@%s:%s?encryption=none#%s", req.SubID, req.Server, port, req.SubID), nil
	case SubscriptionFormatClash:
		return fmt.Sprintf("proxies:\n  - name: %s\n    type: vless\n    server: %s\n    port: %s\n    uuid: %s\n", req.SubID, req.Server, port, req.SubID), nil
	case SubscriptionFormatSingBox:
		return fmt.Sprintf("{\"outbounds\":[{\"type\":\"vless\",\"tag\":\"%s\",\"server\":\"%s\",\"server_port\":%d,\"uuid\":\"%s\"}]}", req.SubID, req.Server, req.Port, req.SubID), nil
	default:
		return "", fmt.Errorf("unsupported format: %s", format)
	}
}

func resolveServerPort(req SubscriptionRequest, inbound models.Inbound, requestHost string) (string, int, error) {
	server := strings.TrimSpace(req.Server)
	if server == "" {
		if tlsMap := asMap(inbound.Options["tls"]); tlsMap != nil {
			server = asString(tlsMap["server_name"])
		}
	}
	if server == "" {
		if host, _, err := net.SplitHostPort(requestHost); err == nil {
			server = host
		} else {
			server = requestHost
		}
	}
	if server == "" {
		return "", 0, fmt.Errorf("cannot resolve server address")
	}

	port := req.Port
	if port == 0 {
		port = inbound.Port
	}
	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("invalid server port")
	}

	return server, port, nil
}

func buildSingBoxSubscriptionConfig(format string, inbound models.Inbound, user models.User, allInbounds []models.Inbound, server string, port int) (string, error) {
	requested := normalizeSubscriptionFormat(format)
	inboundType := strings.ToLower(inbound.Type)

	if requested != SubscriptionFormatSingBox && requested != inboundType {
		return "", fmt.Errorf("requested format %s does not match inbound type %s", requested, inboundType)
	}

	var outbounds []map[string]interface{}
	finalTag := "proxy"

	switch inboundType {
	case "hysteria2":
		proxy := map[string]interface{}{
			"type":        "hysteria2",
			"tag":         "proxy",
			"server":      server,
			"server_port": port,
			"password":    user.UUID,
		}
		if obfs := asMap(inbound.Options["obfs"]); obfs != nil {
			proxy["obfs"] = obfs
		}
		if tlsMap := asMap(inbound.Options["tls"]); tlsMap != nil {
			proxy["tls"] = tlsMap
		}
		outbounds = []map[string]interface{}{proxy}

	case "shadowtls":
		ssTag := asString(inbound.Options["detour"])
		ssInbound, ok := findInboundByTag(allInbounds, ssTag)
		if !ok {
			return "", fmt.Errorf("detour inbound %s not found", ssTag)
		}

		handshakeServer := "www.microsoft.com"
		if handshake := asMap(inbound.Options["handshake"]); handshake != nil {
			handshakeServer = stripPort(asString(handshake["server"]))
		}
		if handshakeServer == "" {
			handshakeServer = "www.microsoft.com"
		}

		version := asInt(inbound.Options["version"])
		if version == 0 {
			version = 3
		}

		st := map[string]interface{}{
			"type":        "shadowtls",
			"tag":         "st-tls",
			"server":      server,
			"server_port": port,
			"version":     version,
			"password":    user.UUID,
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": handshakeServer,
			},
		}

		ss := map[string]interface{}{
			"type":        "shadowsocks",
			"tag":         "proxy",
			"server":      server,
			"server_port": port,
			"method":      asString(ssInbound.Options["method"]),
			"password":    asString(ssInbound.Options["password"]),
			"detour":      "st-tls",
		}

		outbounds = []map[string]interface{}{ss, st}

	case "vless":
		proxy := map[string]interface{}{
			"type":        "vless",
			"tag":         "proxy",
			"server":      server,
			"server_port": port,
			"uuid":        user.UUID,
		}
		if user.Flow != "" {
			proxy["flow"] = user.Flow
		}
		if tlsMap := asMap(inbound.Options["tls"]); tlsMap != nil {
			proxy["tls"] = tlsMap
		}
		if transport := asMap(inbound.Options["transport"]); transport != nil {
			proxy["transport"] = transport
		}
		outbounds = []map[string]interface{}{proxy}

	case "vmess":
		proxy := map[string]interface{}{
			"type":        "vmess",
			"tag":         "proxy",
			"server":      server,
			"server_port": port,
			"uuid":        user.UUID,
		}
		outbounds = []map[string]interface{}{proxy}

	case "trojan", "tuic":
		proxy := map[string]interface{}{
			"type":        inboundType,
			"tag":         "proxy",
			"server":      server,
			"server_port": port,
			"password":    user.UUID,
		}
		outbounds = []map[string]interface{}{proxy}

	default:
		return "", fmt.Errorf("unsupported inbound type for subscription: %s", inboundType)
	}

	outbounds = append(outbounds,
		map[string]interface{}{"type": "direct", "tag": "direct"},
		map[string]interface{}{"type": "block", "tag": "block"},
	)

	root := map[string]interface{}{
		"outbounds": outbounds,
		"route": map[string]interface{}{
			"final": finalTag,
		},
	}

	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to encode subscription config: %w", err)
	}

	return string(b), nil
}

func findInboundByTag(inbounds []models.Inbound, tag string) (models.Inbound, bool) {
	for _, inbound := range inbounds {
		if inbound.Tag == tag {
			return inbound, true
		}
	}
	return models.Inbound{}, false
}

func stripPort(server string) string {
	if host, _, err := net.SplitHostPort(server); err == nil {
		return host
	}
	if i := strings.LastIndex(server, ":"); i > 0 {
		return server[:i]
	}
	return server
}

func asMap(v interface{}) map[string]interface{} {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	return m
}

func asString(v interface{}) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func asInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func (h *SubscriptionHandler) sendSuccess(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := Response{Success: true, Data: data}
	if err := json.NewEncoder(w).Encode(response); err != nil && h.logger != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

func (h *SubscriptionHandler) sendError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := Response{
		Success: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil && h.logger != nil {
		h.logger.Error("failed to encode error response", "error", err)
	}
}
