package internal

import (
	"strings"
	"time"
)

const (
	historyImport     = "import"
	historyDeleteFile = "delete_file"
	historyDeleteItem = "delete_item"
	historyGrab       = "grab"
)

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func (s *Store) appendHistory(itemID, eventType, sourceTitle, quality string) {
	if s == nil || s.db == nil || strings.TrimSpace(itemID) == "" {
		return
	}
	_, _ = s.db.Exec(`INSERT INTO history (item_id, event_type, source_title, quality, date) VALUES (?, ?, ?, ?, ?)`,
		itemID, eventType, sourceTitle, quality, nowRFC3339())
}

func (s *Store) scanHistory(page, pageSize int, itemID, eventType string) ([]map[string]string, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	base := `FROM history`
	var where []string
	var args []any
	if itemID != "" {
		where = append(where, `item_id = ?`)
		args = append(args, itemID)
	}
	if eventType != "" {
		where = append(where, `event_type = ?`)
		args = append(args, eventType)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) `+base+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := s.db.Query(`SELECT item_id, event_type, source_title, quality, date `+base+clause+
		` ORDER BY date DESC LIMIT ? OFFSET ?`, append(append([]any{}, args...), pageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]map[string]string, 0)
	for rows.Next() {
		var id, ev, src, qual, date string
		if err := rows.Scan(&id, &ev, &src, &qual, &date); err != nil {
			return nil, 0, err
		}
		out = append(out, map[string]string{
			"item_id": id, "event_type": ev, "source_title": src, "quality": qual, "date": date,
		})
	}
	return out, total, rows.Err()
}
