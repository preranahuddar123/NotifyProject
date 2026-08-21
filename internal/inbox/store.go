package inbox

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Recipient struct {
	UserID int32  `json:"user_id"`
	Role   string `json:"role"`
}

type Event struct {
	EventID            string          `json:"event_id"`
	LeadID             int32           `json:"lead_id"`
	ProjectID          string          `json:"project_id"`
	LeadName           string          `json:"lead_name"`
	DesignerID         int32           `json:"designer_id"`
	NotificationType   string          `json:"notification_type"`
	NotificationAction string          `json:"notification_action"`
	Payload            json.RawMessage `json:"payload"`
	Recipients         []Recipient     `json:"recipients"`
}

type Row struct {
	ID                 int64
	EventID            string
	UserID             int32
	RecipientRole      string
	LeadID             int32
	ProjectID          string
	LeadName           string
	DesignerID         int32
	NotificationType   string
	NotificationAction string
	Payload            string
	ReadAt             *time.Time
	CreatedAt          time.Time
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) EnsureIndexes() {
	_, _ = s.db.Exec(`CREATE INDEX idx_design_inbox_read_at ON design_user_notifications (read_at)`)
}

func (s *Store) Fanout(evt Event) (userIDs []int32, err error) {
	if strings.TrimSpace(evt.EventID) == "" {
		return nil, fmt.Errorf("event_id is required")
	}
	if strings.TrimSpace(evt.NotificationType) == "" || strings.TrimSpace(evt.NotificationAction) == "" {
		return nil, fmt.Errorf("notification_type and notification_action are required")
	}
	seen := map[int32]struct{}{}
	payload := []byte(evt.Payload)
	if len(payload) == 0 {
		payload = []byte("{}")
	}

	for _, rec := range evt.Recipients {
		if rec.UserID <= 0 {
			continue
		}
		if _, ok := seen[rec.UserID]; ok {
			continue
		}
		seen[rec.UserID] = struct{}{}
		_, execErr := s.db.Exec(
			`INSERT INTO design_user_notifications
			 (event_id, user_id, recipient_role, lead_id, project_id, lead_name, designer_id,
			  notification_type, notification_action, payload, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
			 ON DUPLICATE KEY UPDATE event_id = event_id`,
			evt.EventID,
			rec.UserID,
			rec.Role,
			evt.LeadID,
			evt.ProjectID,
			evt.LeadName,
			evt.DesignerID,
			evt.NotificationType,
			evt.NotificationAction,
			payload,
		)
		if execErr != nil {
			return userIDs, execErr
		}
		userIDs = append(userIDs, rec.UserID)
	}
	return userIDs, nil
}

func (s *Store) List(userID int32, since *time.Time, projectID string, limit int) ([]Row, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	args := []any{userID}
	q := `SELECT id, event_id, user_id, recipient_role, lead_id, project_id, lead_name, designer_id,
	             notification_type, notification_action, payload, read_at, created_at
	      FROM design_user_notifications
	      WHERE user_id = ?`
	if since != nil {
		q += ` AND created_at >= ?`
		args = append(args, *since)
	}
	if strings.TrimSpace(projectID) != "" {
		q += ` AND project_id = ?`
		args = append(args, projectID)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (s *Store) GetForUser(id int64, userID int32) (*Row, error) {
	row := s.db.QueryRow(
		`SELECT id, event_id, user_id, recipient_role, lead_id, project_id, lead_name, designer_id,
		        notification_type, notification_action, payload, read_at, created_at
		 FROM design_user_notifications
		 WHERE id = ? AND user_id = ? LIMIT 1`,
		id, userID,
	)
	item, err := scanRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *Store) Counts(userID int32, since *time.Time) (total int, byType map[string]int, err error) {
	byType = map[string]int{
		"LEAD": 0, "PHASE": 0, "MILESTONE": 0, "PAYMENT": 0, "DQC": 0,
		"MMT": 0, "MEETING": 0, "ASSIGNMENT": 0, "QUOTE": 0, "P2P": 0,
	}
	args := []any{userID}
	q := `SELECT notification_type, COUNT(*) FROM design_user_notifications
	      WHERE user_id = ? AND read_at IS NULL`
	if since != nil {
		q += ` AND created_at >= ?`
		args = append(args, *since)
	}
	q += ` GROUP BY notification_type`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return 0, byType, err
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		var c int
		if err := rows.Scan(&t, &c); err != nil {
			return 0, byType, err
		}
		key := strings.ToUpper(t)
		byType[key] = c
		total += c
	}
	return total, byType, rows.Err()
}

func (s *Store) MarkRead(id int64, userID int32) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE design_user_notifications
		 SET read_at = NOW()
		 WHERE id = ? AND user_id = ? AND read_at IS NULL`,
		id, userID,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) MarkAllRead(userID int32) (int64, error) {
	res, err := s.db.Exec(
		`UPDATE design_user_notifications
		 SET read_at = NOW()
		 WHERE user_id = ? AND read_at IS NULL`,
		userID,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *Store) DeleteExpired() (readDeleted int64, unreadDeleted int64, err error) {
	res, err := s.db.Exec(
		`DELETE FROM design_user_notifications
		 WHERE read_at IS NOT NULL AND read_at < NOW() - INTERVAL 24 HOUR`,
	)
	if err != nil {
		return 0, 0, err
	}
	readDeleted, _ = res.RowsAffected()
	res, err = s.db.Exec(
		`DELETE FROM design_user_notifications
		 WHERE read_at IS NULL AND created_at < NOW() - INTERVAL 30 DAY`,
	)
	if err != nil {
		return readDeleted, 0, err
	}
	unreadDeleted, _ = res.RowsAffected()
	return readDeleted, unreadDeleted, nil
}

func scanRows(rows *sql.Rows) ([]Row, error) {
	out := []Row{}
	for rows.Next() {
		item, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRow(s scanner) (*Row, error) {
	var r Row
	var payload []byte
	var readAt sql.NullTime
	if err := s.Scan(
		&r.ID, &r.EventID, &r.UserID, &r.RecipientRole, &r.LeadID, &r.ProjectID, &r.LeadName,
		&r.DesignerID, &r.NotificationType, &r.NotificationAction, &payload, &readAt, &r.CreatedAt,
	); err != nil {
		return nil, err
	}
	if len(payload) > 0 {
		r.Payload = string(payload)
	} else {
		r.Payload = "{}"
	}
	if readAt.Valid {
		t := readAt.Time
		r.ReadAt = &t
	}
	return &r, nil
}
