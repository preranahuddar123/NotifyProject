package inbox

import (
	"log"
	"time"
)

func StartCleanup(store *Store) {
	store.EnsureIndexes()
	run := func() {
		readN, unreadN, err := store.DeleteExpired()
		if err != nil {
			log.Printf("[inbox-cleanup] failed: %v", err)
			return
		}
		if readN > 0 || unreadN > 0 {
			log.Printf("[inbox-cleanup] deleted read=%d unread=%d", readN, unreadN)
		}
	}
	run()
	go func() {
		t := time.NewTicker(15 * time.Minute)
		defer t.Stop()
		for range t.C {
			run()
		}
	}()
}
