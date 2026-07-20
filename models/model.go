package models

import "time"

// leadDetails table
type LeadDetails struct {
	LeadID    int32     `gorm:"column:lead_id;primaryKey;autoIncrement"`
	LeadName  string    `gorm:"column:lead_name;type:varchar(100);not null"`
	Mobile    int64     `gorm:"column:mobile;uniqueIndex;not null"`
	LeadType  string    `gorm:"column:lead_type;type:enum('SALES_POOL','EXTERNAL_LEAD','GOOGLE_ADS','META_ADS','ADD_LEAD','IVR_CALL','WEBSITE_LEAD','WALK_IN_LEAD','WHATSAPP');not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (LeadDetails) TableName() string { return "leadDetails" }

// meeting_details table
// FK → leadDetails.lead_id
type MeetingDetails struct {
	MeetingID     int32     `gorm:"column:meeting_id;primaryKey;autoIncrement"`
	LeadID        int32     `gorm:"column:lead_id;not null"`
	LeadName      string    `gorm:"column:lead_name;type:varchar(100);not null"`
	Milestone     string    `gorm:"column:milestone;type:enum('SCHEDULED','RESCHEDULED','CANCELLED');not null"`
	Title         string    `gorm:"column:title;type:varchar(150)"`
	Description   string    `gorm:"column:description;type:text"`
	MeetingDate   time.Time `gorm:"column:meeting_date;type:date"`
	Slot          string    `gorm:"column:slot;type:varchar(50)"`
	MeetingType   string    `gorm:"column:meeting_type;type:enum('ONLINE','OFFLINE')"`
	Reason        string    `gorm:"column:reason;type:text"`
	Stage         string    `gorm:"column:stage;type:enum('CONNECTION','EXPERIENCE_AND_DESIGN')"`
	EventDatetime time.Time `gorm:"column:event_datetime;type:datetime"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (MeetingDetails) TableName() string { return "meeting_details" }

// successful table
// FK → leadDetails.lead_id
type Successful struct {
	LeadID             int32     `gorm:"column:lead_id;not null"`
	Description        string    `gorm:"column:description;type:text"`
	Stage              string    `gorm:"column:stage;type:varchar(50)"`
	SuccessDatetime    time.Time `gorm:"column:success_datetime;type:datetime"`
	NextAction         string    `gorm:"column:next_action;type:varchar(100)"`
	QuoteLinkGenerated bool      `gorm:"column:quote_link_generated;default:false"`
	CreatedAt          time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (Successful) TableName() string { return "successful" }

// booking table
// FK → leadDetails.lead_id
type Booking struct {
	BookingID       int32     `gorm:"column:booking_id;primaryKey;autoIncrement"`
	LeadID          int32     `gorm:"column:lead_id;not null"`
	PaymentType     string    `gorm:"column:payment_type;type:enum('TOKEN','BOOKING FUll_10');not null"`
	PaidAmount      float64   `gorm:"column:paid_amount;type:decimal(10,2);not null"`
	RemainingAmount float64   `gorm:"column:Remaining_amount;type:decimal(10,2);not null"`
	PaymentDate     time.Time `gorm:"column:payment_date;type:datetime;not null"`
	PaymentStatus   string    `gorm:"column:payment_status;type:enum('PENDING','SUCCESS','FAILED');default:'PENDING'"`
	Remarks         string    `gorm:"column:remarks;type:text"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (Booking) TableName() string { return "booking" }

// Notification maps to the `notification` table.
type Notification struct {
	NotificationID int32     `gorm:"column:notification_id;primaryKey;autoIncrement"`
	LeadID         int32     `gorm:"column:lead_id;not null"`
	SourceModule   string    `gorm:"column:source_module;type:enum('CRM');default:'CRM'"`
	EventType      string    `gorm:"column:event_type;type:varchar(50);not null"`
	RefTable       string    `gorm:"column:ref_table;type:varchar(50);not null"`
	RefID          int32     `gorm:"column:ref_id;not null"`
	IdempotencyKey string    `gorm:"column:idempotency_key;type:varchar(255);uniqueIndex;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (Notification) TableName() string { return "notification" }

// NotificationRecipient maps to the `notification_recipients` table.
type NotificationRecipient struct {
	RecipientID         int32     `gorm:"column:recipient_id;primaryKey;autoIncrement"`
	NotificationID      int32     `gorm:"column:notification_id;not null"`
	LeadID              int32     `gorm:"column:lead_id;not null"`
	RecipientType       string    `gorm:"column:recipient_type;type:varchar(50);not null"`
	RecipientIdentifier string    `gorm:"column:recipient_identifier;type:varchar(100);not null"`
	DeliveryStatus      string    `gorm:"column:delivery_status;type:varchar(20);default:'PENDING'"`
	SentAt              time.Time `gorm:"column:sent_at;type:timestamp;default:null"`
}

func (NotificationRecipient) TableName() string { return "notification_recipients" }
