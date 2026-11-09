package storage

import "time"

// AnnouncementRecord is an independently managed platform post.
type AnnouncementRecord struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Content     string     `json:"content"`
	Level       string     `json:"level"`
	Status      string     `json:"status"`
	ShowBanner  bool       `json:"show_banner"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	PublishedAt *time.Time `json:"published_at"`
}

type AnnouncementPage struct {
	Items []AnnouncementRecord `json:"items"`
	Total int64                `json:"total"`
}

type NotificationRecord struct {
	ID              string     `json:"id"`
	Kind            string     `json:"kind"`
	AnnouncementID  string     `json:"announcement_id,omitempty"`
	Title           string     `json:"title"`
	Content         string     `json:"content"`
	Level           string     `json:"level"`
	RecipientUserID *int64     `json:"-"`
	CreatedAt       time.Time  `json:"created_at"`
	ReadAt          *time.Time `json:"read_at"`
}

type NotificationFilter struct {
	Kind     string
	Unread   bool
	Page     int
	PageSize int
}

type NotificationPage struct {
	Items          []NotificationRecord `json:"items"`
	Total          int64                `json:"total"`
	LatestSequence int64                `json:"latest_sequence"`
	Page           int                  `json:"page"`
	PageSize       int                  `json:"page_size"`
	NotificationCounts
}

type NotificationCounts struct {
	UnreadCount             int64 `json:"unread_count"`
	AnnouncementUnreadCount int64 `json:"announcement_unread_count"`
	MessageUnreadCount      int64 `json:"message_unread_count"`
}
