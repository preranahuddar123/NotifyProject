package inbox

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type ticketEntry struct {
	UserID    int32
	ExpiresAt time.Time
}

type TicketStore struct {
	mu    sync.Mutex
	items map[string]ticketEntry
}

func NewTicketStore() *TicketStore {
	s := &TicketStore{items: map[string]ticketEntry{}}
	go s.gc()
	return s
}

func (s *TicketStore) Issue(userID int32, ttl time.Duration) (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	ticket := hex.EncodeToString(buf)
	s.mu.Lock()
	s.items[ticket] = ticketEntry{UserID: userID, ExpiresAt: time.Now().Add(ttl)}
	s.mu.Unlock()
	return ticket, nil
}

func (s *TicketStore) Take(ticket string) (int32, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.items[ticket]
	if !ok {
		return 0, false
	}
	delete(s.items, ticket)
	if time.Now().After(entry.ExpiresAt) {
		return 0, false
	}
	return entry.UserID, true
}

func (s *TicketStore) gc() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		s.mu.Lock()
		for k, v := range s.items {
			if now.After(v.ExpiresAt) {
				delete(s.items, k)
			}
		}
		s.mu.Unlock()
	}
}
