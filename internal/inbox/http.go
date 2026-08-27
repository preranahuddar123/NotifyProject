package inbox

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type HTTP struct {
	store   *Store
	hub     *Hub
	tickets *TicketStore
}

var DefaultHub = NewHub()
var DefaultTicketStore = NewTicketStore()

func NewHTTP(store *Store) *HTTP {
	return &HTTP{
		store:   store,
		hub:     DefaultHub,
		tickets: DefaultTicketStore,
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			return true
		}
		allow := strings.TrimSpace(os.Getenv("NOTIFY_WS_ORIGINS"))
		if allow == "" || allow == "*" {
			return true
		}
		for _, o := range strings.Split(allow, ",") {
			if strings.TrimSpace(o) == origin {
				return true
			}
		}
		return strings.Contains(origin, "localhost") || strings.Contains(origin, "127.0.0.1")
	},
}

func (h *HTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/design/inbox")
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		path = "/"
	}

	if r.Method == http.MethodGet && path == "/ws" {
		h.ws(w, r)
		return
	}

	if !h.authorized(r) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	switch {
	case r.Method == http.MethodPost && path == "/events":
		h.createEvent(w, r)
	case r.Method == http.MethodPost && path == "/ws-ticket":
		h.wsTicket(w, r)
	case r.Method == http.MethodPost && path == "/read-all":
		h.markAllRead(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/read") && isNumericID(strings.TrimSuffix(path, "/read")):
		h.markRead(w, r, strings.TrimPrefix(strings.TrimSuffix(path, "/read"), "/"))
	case r.Method == http.MethodGet && path == "/counts":
		h.counts(w, r)
	case r.Method == http.MethodGet && path == "/":
		h.list(w, r)
	case r.Method == http.MethodGet && isNumericID(path):
		h.detail(w, r, strings.TrimPrefix(path, "/"))
	default:
		writeErr(w, http.StatusNotFound, "not found")
	}
}

func (h *HTTP) authorized(r *http.Request) bool {
	want := strings.TrimSpace(os.Getenv("NOTIFY_API_KEY"))
	if want == "" {
		return true
	}
	got := strings.TrimSpace(r.Header.Get("x-external-api-key"))
	return got == want
}

func (h *HTTP) createEvent(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	var evt Event
	if err := json.Unmarshal(body, &evt); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if key := strings.TrimSpace(r.Header.Get("Idempotency-Key")); key != "" && evt.EventID == "" {
		evt.EventID = key
	}
	userIDs, err := h.store.Fanout(evt)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	h.hub.Broadcast(userIDs, map[string]any{
		"type":     "inbox_updated",
		"event_id": evt.EventID,
	})
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":         true,
		"event_id":   evt.EventID,
		"recipients": len(userIDs),
	})
}

func (h *HTTP) wsTicket(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	var req struct {
		UserID int32 `json:"user_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.UserID <= 0 {
		writeErr(w, http.StatusBadRequest, "user_id is required")
		return
	}
	ticket, err := h.tickets.Issue(req.UserID, 60*time.Second)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to issue ticket")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":     ticket,
		"expires_in": 60,
	})
}

func (h *HTTP) ws(w http.ResponseWriter, r *http.Request) {
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	userID, ok := h.tickets.Take(ticket)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "invalid or expired ticket")
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.hub.Add(userID, conn)
	defer func() {
		h.hub.Remove(userID, conn)
		_ = conn.Close()
	}()
	_ = conn.WriteJSON(map[string]any{"type": "connected", "user_id": userID})
	conn.SetReadLimit(4096)
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *HTTP) list(w http.ResponseWriter, r *http.Request) {
	userID, err := queryUserID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "user_id is required")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	since := parseSince(r.URL.Query().Get("since"))
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	rows, err := h.store.List(userID, since, projectID, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list inbox")
		return
	}
	data := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, rowToJSON(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (h *HTTP) counts(w http.ResponseWriter, r *http.Request) {
	userID, err := queryUserID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "user_id is required")
		return
	}
	since := parseSince(r.URL.Query().Get("since"))
	total, byType, err := h.store.Counts(userID, since)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to count inbox")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"total":   total,
			"by_type": byType,
		},
	})
}

func (h *HTTP) detail(w http.ResponseWriter, r *http.Request, idStr string) {
	userID, err := queryUserID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "user_id is required")
		return
	}
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	row, err := h.store.GetForUser(id, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to load inbox item")
		return
	}
	if row == nil {
		writeErr(w, http.StatusNotFound, "notification not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rowToJSON(*row)})
}

func (h *HTTP) markRead(w http.ResponseWriter, r *http.Request, idStr string) {
	userID, err := queryUserID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "user_id is required")
		return
	}
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	row, err := h.store.GetForUser(id, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to mark read")
		return
	}
	if row == nil {
		writeErr(w, http.StatusNotFound, "notification not found")
		return
	}
	_, err = h.store.MarkRead(id, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to mark read")
		return
	}
	h.hub.Broadcast([]int32{userID}, map[string]any{"type": "inbox_updated", "reason": "read"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTP) markAllRead(w http.ResponseWriter, r *http.Request) {
	userID, err := queryUserID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "user_id is required")
		return
	}
	n, err := h.store.MarkAllRead(userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to mark all read")
		return
	}
	h.hub.Broadcast([]int32{userID}, map[string]any{"type": "inbox_updated", "reason": "read-all"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": n})
}

func queryUserID(r *http.Request) (int32, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if raw == "" {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if err == nil && len(body) > 0 {
			var req struct {
				UserID int32 `json:"user_id"`
			}
			if json.Unmarshal(body, &req) == nil && req.UserID > 0 {
				return req.UserID, nil
			}
		}
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("user_id is required")
	}
	return int32(n), nil
}

func parseSince(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t2, err2 := time.Parse("2006-01-02", raw)
		if err2 != nil {
			return nil
		}
		t = t2
	}
	return &t
}

func isNumericID(path string) bool {
	if !strings.HasPrefix(path, "/") {
		return false
	}
	_, err := strconv.Atoi(strings.TrimPrefix(path, "/"))
	return err == nil
}

func rowToJSON(row Row) map[string]any {
	var payload any
	if err := json.Unmarshal([]byte(row.Payload), &payload); err != nil {
		payload = map[string]any{}
	}
	return map[string]any{
		"id":                  row.ID,
		"event_id":            row.EventID,
		"user_id":             row.UserID,
		"recipient_role":      row.RecipientRole,
		"project_id":          row.ProjectID,
		"lead_id":             row.LeadID,
		"lead_name":           row.LeadName,
		"designer_id":         row.DesignerID,
		"notification_type":   row.NotificationType,
		"notification_action": row.NotificationAction,
		"payload":             payload,
		"created_at":          row.CreatedAt.UTC().Format(time.RFC3339),
		"read_at":             formatTime(row.ReadAt),
	}
}

func formatTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}
