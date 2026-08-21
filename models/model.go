package models

import "time"

// leadDetails table
type LeadDetails struct {
	LeadIdentifier string    `gorm:"column:lead_identifier;primaryKey;type:varchar(100)"`
	LeadName       string    `gorm:"column:lead_name;type:varchar(100);not null"`
	LeadType       string    `gorm:"column:lead_type;type:enum('SALES_POOL','EXTERNAL_LEAD','GOOGLE_ADS','META_ADS','ADD_LEAD','IVR_CALL','WEBSITE_LEAD','WALK_IN_LEAD','WHATSAPP');not null"`
	AssignedTo     string    `gorm:"column:assigned_to;type:varchar(100)"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (LeadDetails) TableName() string { return "leadDetails" }

// meeting_details table — standalone, no foreign key constraints
type MeetingDetails struct {
	MeetingID      int32     `gorm:"column:meeting_id;primaryKey;autoIncrement"`
	LeadIdentifier string    `gorm:"column:lead_identifier;type:varchar(100);not null"`
	LeadName       string    `gorm:"column:lead_name;type:varchar(100);not null"`
	AssignedTo     string    `gorm:"column:assigned_to;type:varchar(100)"`
	SubStage       string    `gorm:"column:sub_stage;type:enum('SCHEDULED','RESCHEDULED','CANCELLED');not null"`
	Title          string    `gorm:"column:title;type:varchar(150)"`
	MeetingDate    time.Time `gorm:"column:meeting_date;type:date"`
	Slot           string    `gorm:"column:slot;type:varchar(50)"`
	MeetingType    string    `gorm:"column:meeting_type;type:enum('VIRTUAL_MEETING','SHOWROOM_VISIT','SITE_VISIT');not null"`
	Milestone      string    `gorm:"column:milestone;type:enum('CONNECTION','EXPERIENCE_AND_DESIGN')"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
}

// Note: reason column has been removed; sub_stage replaces submilestone; milestone replaces stage

func (MeetingDetails) TableName() string { return "meeting_details" }

// successful table — standalone, no foreign key constraints
type Successful struct {
	LeadIdentifier     string    `gorm:"column:lead_identifier;type:varchar(100);not null"`
	LeadName           string    `gorm:"column:lead_name;type:varchar(100);not null"`
	AssignedTo         string    `gorm:"column:assigned_to;type:varchar(100)"`
	NextAction         string    `gorm:"column:next_action;type:varchar(100)"`
	QuoteLinkGenerated bool      `gorm:"column:quote_link_generated;default:false"`
	CreatedAt          time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (Successful) TableName() string { return "successful" }

// booking table — standalone, no foreign key constraints
type Booking struct {
	BookingID       int32     `gorm:"column:booking_id;primaryKey;autoIncrement"`
	LeadName        string    `gorm:"column:lead_name;type:varchar(100);not null"`
	LeadIdentifier  string    `gorm:"column:lead_identifier;type:varchar(100);not null"`
	AssignedTo      string    `gorm:"column:assigned_to;type:varchar(100)"`
	PaymentType     string    `gorm:"column:payment_type;type:enum('TOKEN','BOOKING FUll_10');not null"`
	PaidAmount      float64   `gorm:"column:paid_amount;type:decimal(10,2);not null"`
	RemainingAmount float64   `gorm:"column:Remaining_amount;type:decimal(10,2);not null"`
	PaymentDate     time.Time `gorm:"column:payment_date;type:datetime;not null"`
	PaymentStatus   string    `gorm:"column:payment_status;type:enum('PENDING','SUCCESS','FAILED');default:'PENDING'"`
	Remarks         string    `gorm:"column:remarks;type:text"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (Booking) TableName() string { return "booking" }

// notification table — standalone, no foreign key constraints
type Notification struct {
	NotificationID int32     `gorm:"column:notification_id;primaryKey;autoIncrement"`
	LeadIdentifier string    `gorm:"column:lead_identifier;type:varchar(100);not null"`
	SourceModule   string    `gorm:"column:source_module;type:enum('CRM');default:'CRM'"`
	EventType      string    `gorm:"column:event_type;type:varchar(50);not null"`
	RefTable       string    `gorm:"column:ref_table;type:varchar(50);not null"`
	RefID          int32     `gorm:"column:ref_id;not null"`
	IdempotencyKey string    `gorm:"column:idempotency_key;type:varchar(255);uniqueIndex;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (Notification) TableName() string { return "notification" }

// notification_recipients table — standalone, no foreign key constraints
type NotificationRecipient struct {
	RecipientID    int32     `gorm:"column:recipient_id;primaryKey;autoIncrement"`
	NotificationID int32     `gorm:"column:notification_id;not null"`
	UserID         int32     `gorm:"column:user_id;not null"`
	RecipientType  string    `gorm:"column:recipient_type;type:varchar(50);not null"`
	DeliveryStatus string    `gorm:"column:delivery_status;type:varchar(20);default:'PENDING'"`
	SeenAt         time.Time `gorm:"column:seen_at;type:timestamp;default:null"`
}

func (NotificationRecipient) TableName() string { return "notification_recipients" }

// DesignUserNotification is one inbox row per person (fan-out on write).
type DesignUserNotification struct {
	ID                 int64      `gorm:"column:id;primaryKey;autoIncrement"`
	EventID            string     `gorm:"column:event_id;type:varchar(255);uniqueIndex:uk_design_inbox_event_user;not null"`
	UserID             int32      `gorm:"column:user_id;uniqueIndex:uk_design_inbox_event_user;index:idx_design_inbox_user_created;not null"`
	RecipientRole      string     `gorm:"column:recipient_role;type:varchar(50)"`
	LeadID             int32      `gorm:"column:lead_id"`
	ProjectID          string     `gorm:"column:project_id;type:varchar(100)"`
	LeadName           string     `gorm:"column:lead_name;type:varchar(255)"`
	DesignerID         int32      `gorm:"column:designer_id"`
	NotificationType   string     `gorm:"column:notification_type;type:varchar(30);not null"`
	NotificationAction string     `gorm:"column:notification_action;type:varchar(30);not null"`
	Payload            string     `gorm:"column:payload;type:json"`
	ReadAt             *time.Time `gorm:"column:read_at;index:idx_design_inbox_read_at"`
	CreatedAt          time.Time  `gorm:"column:created_at;autoCreateTime;index:idx_design_inbox_user_created"`
}

func (DesignUserNotification) TableName() string { return "design_user_notifications" }
