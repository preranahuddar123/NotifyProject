# Design inbox (Go)

The Design Module bell is documented here:

**[NOTIFICATION_SYSTEM.md](../../FrontandProjects/DesignModulephase1/NOTIFICATION_SYSTEM.md)**

Live HTTP for Design (not the old proto stubs):

- `POST /v1/design/inbox/events`
- `POST /v1/design/inbox/ws-ticket`
- `POST /v1/design/inbox/read-all?user_id=`
- `POST /v1/design/inbox/{id}/read?user_id=`
- `GET /v1/design/inbox?user_id=`
- `GET /v1/design/inbox/counts?user_id=` (unread: `read_at IS NULL`)
- `GET /v1/design/inbox/{id}?user_id=`
- `WS /v1/design/inbox/ws?ticket=`

Cleanup: every 15 minutes, delete read rows older than 24 hours after `read_at`, and unread rows older than 30 days after `created_at`.

Code: `internal/inbox/http.go`, `hub.go`, `tickets.go`, `store.go`, `cleanup.go`, table `design_user_notifications`.
