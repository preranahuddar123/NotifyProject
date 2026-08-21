package inbox

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

type Hub struct {
	mu    sync.RWMutex
	conns map[int32]map[*websocket.Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{conns: map[int32]map[*websocket.Conn]struct{}{}}
}

func (h *Hub) Add(userID int32, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[userID] == nil {
		h.conns[userID] = map[*websocket.Conn]struct{}{}
	}
	h.conns[userID][conn] = struct{}{}
}

func (h *Hub) Remove(userID int32, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.conns[userID]
	if set == nil {
		return
	}
	delete(set, conn)
	if len(set) == 0 {
		delete(h.conns, userID)
	}
}

func (h *Hub) Broadcast(userIDs []int32, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	seen := map[*websocket.Conn]struct{}{}
	for _, userID := range userIDs {
		for conn := range h.conns[userID] {
			if _, ok := seen[conn]; ok {
				continue
			}
			seen[conn] = struct{}{}
			_ = conn.WriteMessage(websocket.TextMessage, body)
		}
	}
}
