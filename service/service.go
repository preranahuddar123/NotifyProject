package service

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "NotifyProject/proto/protogen/notify"
)

type NotifyServiceServer struct {
	pb.UnimplementedNotifyServiceServer
	db *sql.DB
}

func NewNotifyServiceServer(db *sql.DB) *NotifyServiceServer {
	return &NotifyServiceServer{db: db}
}

func stageDBValue(s pb.MeetingStage) string {
	switch s {
	case pb.MeetingStage_STAGE_CONNECTION:
		return "CONNECTION"
	case pb.MeetingStage_STAGE_EXPERIENCE_DESIGN:
		return "EXPERIENCE_AND_DESIGN"
	default:
		return ""
	}
}

func stageProtoValue(s string) pb.MeetingStage {
	switch s {
	case "CONNECTION":
		return pb.MeetingStage_STAGE_CONNECTION
	case "EXPERIENCE_AND_DESIGN":
		return pb.MeetingStage_STAGE_EXPERIENCE_DESIGN
	default:
		return pb.MeetingStage_STAGE_UNSPECIFIED
	}
}

func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339, s) }
func fmtTime(t time.Time) string            { return t.Format(time.RFC3339) }

// Cancellation RPCs

func (s *NotifyServiceServer) CreateCancellation(_ context.Context, req *pb.CreateCancellationRequest) (*pb.CancellationResponse, error) {
	if req.LeadId == 0 || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: lead_id and lead_name are required")
	}
	if req.EventDatetime == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: event_datetime is required (RFC3339)")
	}
	if req.Stage == pb.MeetingStage_STAGE_UNSPECIFIED {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: stage must be STAGE_CONNECTION or STAGE_EXPERIENCE_DESIGN")
	}
	dt, err := parseTime(req.EventDatetime)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: %v", err)
	}
	res, err := s.db.Exec(
		`INSERT INTO meeting_details (lead_id, lead_name, milestone, reason, stage, event_datetime) VALUES (?, ?, 'CANCELLED', ?, ?, ?)`,
		req.LeadId, req.LeadName, req.Reason, stageDBValue(req.Stage), dt,
	)
	if err != nil {
		log.Printf("Failed to create cancellation: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to create cancellation: %v", err)
	}
	id, _ := res.LastInsertId()
	return s.getCancellationByID(int32(id))
}

func (s *NotifyServiceServer) GetAllCancellations(_ context.Context, req *pb.GetByLeadIDRequest) (*pb.CancellationListResponse, error) {
	var rows *sql.Rows
	var err error
	if req.LeadId > 0 {
		rows, err = s.db.Query(
			`SELECT meeting_id, lead_id, lead_name, reason, stage, event_datetime, created_at
			 FROM meeting_details WHERE milestone = 'CANCELLED' AND lead_id = ? ORDER BY created_at DESC`, req.LeadId)
	} else {
		rows, err = s.db.Query(
			`SELECT meeting_id, lead_id, lead_name, reason, stage, event_datetime, created_at
			 FROM meeting_details WHERE milestone = 'CANCELLED' ORDER BY created_at DESC`)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch cancellations: %v", err)
	}
	defer rows.Close()
	var list []*pb.MeetingCancellation
	for rows.Next() {
		item, e := scanCancellation(rows.Scan)
		if e != nil {
			return nil, status.Errorf(codes.Internal, "scan error: %v", e)
		}
		list = append(list, item)
	}
	return &pb.CancellationListResponse{Data: list}, rows.Err()
}

func (s *NotifyServiceServer) GetCancellationByID(_ context.Context, req *pb.GetByMeetingIDRequest) (*pb.CancellationResponse, error) {
	if req.MeetingId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: meeting_id is required")
	}
	return s.getCancellationByID(req.MeetingId)
}

// Success RPCs

func (s *NotifyServiceServer) CreateSuccess(_ context.Context, req *pb.CreateSuccessRequest) (*pb.SuccessResponse, error) {
	if req.LeadId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: lead_id is required")
	}
	if req.SuccessDatetime == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: success_datetime is required (RFC3339)")
	}
	if req.NextAction == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: next_action is required")
	}
	if req.Stage == pb.MeetingStage_STAGE_UNSPECIFIED {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: stage must be STAGE_CONNECTION or STAGE_EXPERIENCE_DESIGN")
	}
	dt, err := parseTime(req.SuccessDatetime)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: %v", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO successful (lead_id, description, stage, success_datetime, next_action, quote_link_generated) VALUES (?, ?, ?, ?, ?, ?)`,
		req.LeadId, req.Description, stageDBValue(req.Stage), dt, req.NextAction, req.QuoteLinkGenerated,
	)
	if err != nil {
		log.Printf("Failed to create success record: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to create success record: %v", err)
	}
	return s.getSuccessByLeadID(req.LeadId)
}

func (s *NotifyServiceServer) GetAllSuccesses(_ context.Context, req *pb.GetByLeadIDRequest) (*pb.SuccessListResponse, error) {
	const base = `SELECT s.lead_id, l.lead_name, s.description, s.stage,
		s.success_datetime, s.next_action, s.quote_link_generated, s.created_at
		FROM successful s JOIN leadDetails l ON l.lead_id = s.lead_id`
	var rows *sql.Rows
	var err error
	if req.LeadId > 0 {
		rows, err = s.db.Query(base+` WHERE s.lead_id = ? ORDER BY s.created_at DESC`, req.LeadId)
	} else {
		rows, err = s.db.Query(base + ` ORDER BY s.created_at DESC`)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch success records: %v", err)
	}
	defer rows.Close()
	var list []*pb.MeetingSuccess
	for rows.Next() {
		item, e := scanSuccess(rows.Scan)
		if e != nil {
			return nil, status.Errorf(codes.Internal, "scan error: %v", e)
		}
		list = append(list, item)
	}
	return &pb.SuccessListResponse{Data: list}, rows.Err()
}

func (s *NotifyServiceServer) GetSuccessByLeadID(_ context.Context, req *pb.GetByLeadIDRequest) (*pb.SuccessResponse, error) {
	if req.LeadId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: lead_id is required")
	}
	return s.getSuccessByLeadID(req.LeadId)
}

// Scheduled RPCs

func (s *NotifyServiceServer) CreateScheduled(_ context.Context, req *pb.CreateScheduledRequest) (*pb.ScheduledResponse, error) {
	if req.LeadId == 0 || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: lead_id and lead_name are required")
	}
	if req.MeetingDate == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: meeting_date is required")
	}
	if req.Slot == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: slot is required")
	}
	if req.MeetingType == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: meeting_type is required")
	}
	if req.Stage == pb.MeetingStage_STAGE_UNSPECIFIED {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: stage is required")
	}
	res, err := s.db.Exec(
		`INSERT INTO meeting_details (lead_id, lead_name, milestone, title, description, meeting_date, slot, meeting_type, stage)
		 VALUES (?, ?, 'SCHEDULED', ?, ?, ?, ?, ?, ?)`,
		req.LeadId, req.LeadName, req.Title, req.Description,
		req.MeetingDate, req.Slot, req.MeetingType, stageDBValue(req.Stage),
	)
	if err != nil {
		log.Printf("Failed to create scheduled meeting: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to create scheduled meeting: %v", err)
	}
	id, _ := res.LastInsertId()
	return s.getScheduledByID(int32(id))
}

func (s *NotifyServiceServer) GetAllScheduled(_ context.Context, req *pb.GetScheduledByLeadIDRequest) (*pb.ScheduledListResponse, error) {
	var rows *sql.Rows
	var err error
	if req.LeadId != 0 {
		rows, err = s.db.Query(
			`SELECT meeting_id, lead_id, lead_name, title, description, meeting_date, slot, meeting_type, stage, created_at
			 FROM meeting_details WHERE milestone = 'SCHEDULED' AND lead_id = ? ORDER BY created_at DESC`, req.LeadId)
	} else {
		rows, err = s.db.Query(
			`SELECT meeting_id, lead_id, lead_name, title, description, meeting_date, slot, meeting_type, stage, created_at
			 FROM meeting_details WHERE milestone = 'SCHEDULED' ORDER BY created_at DESC`)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch scheduled meetings: %v", err)
	}
	defer rows.Close()
	var list []*pb.MeetingScheduled
	for rows.Next() {
		item, e := scanScheduled(rows.Scan)
		if e != nil {
			return nil, status.Errorf(codes.Internal, "scan error: %v", e)
		}
		list = append(list, item)
	}
	return &pb.ScheduledListResponse{Data: list}, rows.Err()
}

func (s *NotifyServiceServer) GetScheduledByID(_ context.Context, req *pb.GetScheduledByIDRequest) (*pb.ScheduledResponse, error) {
	if req.MeetingId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: meeting_id is required")
	}
	return s.getScheduledByID(req.MeetingId)
}

// Rescheduled RPCs

func (s *NotifyServiceServer) CreateRescheduled(_ context.Context, req *pb.CreateRescheduledRequest) (*pb.RescheduledResponse, error) {
	if req.LeadId == 0 || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: lead_id and lead_name are required")
	}
	if req.MeetingDate == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: meeting_date is required")
	}
	if req.Slot == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: slot is required")
	}
	if req.MeetingType == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: meeting_type is required")
	}
	if req.Stage == pb.MeetingStage_STAGE_UNSPECIFIED {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: stage is required")
	}
	if req.Reason == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: reason is required")
	}
	res, err := s.db.Exec(
		`INSERT INTO meeting_details (lead_id, lead_name, milestone, title, description, meeting_date, slot, meeting_type, stage, reason)
		 VALUES (?, ?, 'RESCHEDULED', ?, ?, ?, ?, ?, ?, ?)`,
		req.LeadId, req.LeadName, req.Title, req.Description,
		req.MeetingDate, req.Slot, req.MeetingType, stageDBValue(req.Stage), req.Reason,
	)
	if err != nil {
		log.Printf("Failed to create rescheduled meeting: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to create rescheduled meeting: %v", err)
	}
	id, _ := res.LastInsertId()
	return s.getRescheduledByID(int32(id))
}

func (s *NotifyServiceServer) GetAllRescheduled(_ context.Context, req *pb.GetRescheduledByLeadIDRequest) (*pb.RescheduledListResponse, error) {
	var rows *sql.Rows
	var err error
	if req.LeadId != 0 {
		rows, err = s.db.Query(
			`SELECT meeting_id, lead_id, lead_name, title, description, meeting_date, slot, meeting_type, stage, reason, created_at
			 FROM meeting_details WHERE milestone = 'RESCHEDULED' AND lead_id = ? ORDER BY created_at DESC`, req.LeadId)
	} else {
		rows, err = s.db.Query(
			`SELECT meeting_id, lead_id, lead_name, title, description, meeting_date, slot, meeting_type, stage, reason, created_at
			 FROM meeting_details WHERE milestone = 'RESCHEDULED' ORDER BY created_at DESC`)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch rescheduled meetings: %v", err)
	}
	defer rows.Close()
	var list []*pb.MeetingRescheduled
	for rows.Next() {
		item, e := scanRescheduled(rows.Scan)
		if e != nil {
			return nil, status.Errorf(codes.Internal, "scan error: %v", e)
		}
		list = append(list, item)
	}
	return &pb.RescheduledListResponse{Data: list}, rows.Err()
}

func (s *NotifyServiceServer) GetRescheduledByID(_ context.Context, req *pb.GetRescheduledByIDRequest) (*pb.RescheduledResponse, error) {
	if req.MeetingId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: meeting_id is required")
	}
	return s.getRescheduledByID(req.MeetingId)
}

// Private fetch helpers

func (s *NotifyServiceServer) getCancellationByID(meetingID int32) (*pb.CancellationResponse, error) {
	row := s.db.QueryRow(
		`SELECT meeting_id, lead_id, lead_name, reason, stage, event_datetime, created_at
		 FROM meeting_details WHERE meeting_id = ? AND milestone = 'CANCELLED'`, meetingID)
	item, err := scanCancellation(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "Cancellation with meeting_id %d not found", meetingID)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch cancellation: %v", err)
	}
	return &pb.CancellationResponse{Data: item}, nil
}

func (s *NotifyServiceServer) getSuccessByLeadID(leadID int32) (*pb.SuccessResponse, error) {
	row := s.db.QueryRow(
		`SELECT s.lead_id, l.lead_name, s.description, s.stage,
		        s.success_datetime, s.next_action, s.quote_link_generated, s.created_at
		 FROM successful s JOIN leadDetails l ON l.lead_id = s.lead_id
		 WHERE s.lead_id = ? LIMIT 1`, leadID)
	item, err := scanSuccess(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "Success record for lead_id %d not found", leadID)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch success record: %v", err)
	}
	return &pb.SuccessResponse{Data: item}, nil
}

func (s *NotifyServiceServer) getScheduledByID(id int32) (*pb.ScheduledResponse, error) {
	row := s.db.QueryRow(
		`SELECT meeting_id, lead_id, lead_name, title, description, meeting_date, slot, meeting_type, stage, created_at
		 FROM meeting_details WHERE meeting_id = ? AND milestone = 'SCHEDULED'`, id)
	item, err := scanScheduled(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "Scheduled meeting with meeting_id %d not found", id)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch scheduled meeting: %v", err)
	}
	return &pb.ScheduledResponse{Data: item}, nil
}

func (s *NotifyServiceServer) getRescheduledByID(id int32) (*pb.RescheduledResponse, error) {
	row := s.db.QueryRow(
		`SELECT meeting_id, lead_id, lead_name, title, description, meeting_date, slot, meeting_type, stage, reason, created_at
		 FROM meeting_details WHERE meeting_id = ? AND milestone = 'RESCHEDULED'`, id)
	item, err := scanRescheduled(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "Rescheduled meeting with meeting_id %d not found", id)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch rescheduled meeting: %v", err)
	}
	return &pb.RescheduledResponse{Data: item}, nil
}

// Row scanners

func scanCancellation(fn func(...any) error) (*pb.MeetingCancellation, error) {
	var (
		m         pb.MeetingCancellation
		reason    sql.NullString
		stage     string
		eventTime *time.Time
		createdAt *time.Time
	)
	if err := fn(&m.MeetingId, &m.LeadId, &m.LeadName, &reason, &stage, &eventTime, &createdAt); err != nil {
		return nil, err
	}
	m.Reason = reason.String
	m.Stage = stageProtoValue(stage)
	if eventTime != nil {
		m.EventDatetime = fmtTime(*eventTime)
	}
	if createdAt != nil {
		m.CreatedAt = fmtTime(*createdAt)
	}
	return &m, nil
}

func scanSuccess(fn func(...any) error) (*pb.MeetingSuccess, error) {
	var (
		m           pb.MeetingSuccess
		description sql.NullString
		stage       string
		nextAction  sql.NullString
		successDt   *time.Time
		createdAt   *time.Time
	)
	if err := fn(&m.LeadId, &m.LeadName, &description, &stage, &successDt, &nextAction, &m.QuoteLinkGenerated, &createdAt); err != nil {
		return nil, err
	}
	m.Description = description.String
	m.NextAction = nextAction.String
	m.Stage = stageProtoValue(stage)
	if successDt != nil {
		m.SuccessDatetime = fmtTime(*successDt)
	}
	if createdAt != nil {
		m.CreatedAt = fmtTime(*createdAt)
	}
	return &m, nil
}

func scanScheduled(fn func(...any) error) (*pb.MeetingScheduled, error) {
	var (
		m           pb.MeetingScheduled
		title       sql.NullString
		description sql.NullString
		meetingDate sql.NullString
		slot        sql.NullString
		meetingType sql.NullString
		stage       string
		createdAt   time.Time
	)
	if err := fn(&m.Id, &m.LeadId, &m.LeadName, &title, &description, &meetingDate, &slot, &meetingType, &stage, &createdAt); err != nil {
		return nil, err
	}
	m.Title = title.String
	m.Description = description.String
	m.MeetingDate = meetingDate.String
	m.Slot = slot.String
	m.MeetingType = meetingType.String
	m.Stage = stageProtoValue(stage)
	m.CreatedAt = fmtTime(createdAt)
	return &m, nil
}

func scanRescheduled(fn func(...any) error) (*pb.MeetingRescheduled, error) {
	var (
		m           pb.MeetingRescheduled
		title       sql.NullString
		description sql.NullString
		meetingDate sql.NullString
		slot        sql.NullString
		meetingType sql.NullString
		stage       string
		reason      sql.NullString
		createdAt   time.Time
	)
	if err := fn(&m.Id, &m.LeadId, &m.LeadName, &title, &description, &meetingDate, &slot, &meetingType, &stage, &reason, &createdAt); err != nil {
		return nil, err
	}
	m.Title = title.String
	m.Description = description.String
	m.MeetingDate = meetingDate.String
	m.Slot = slot.String
	m.MeetingType = meetingType.String
	m.Stage = stageProtoValue(stage)
	m.Reason = reason.String
	m.CreatedAt = fmtTime(createdAt)
	return &m, nil
}

// Lead RPCs

func (s *NotifyServiceServer) CreateLead(_ context.Context, req *pb.CreateLeadRequest) (*pb.LeadResponse, error) {
	if req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: lead_name is required")
	}
	if req.Mobile == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: mobile is required")
	}
	if req.LeadType == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: lead_type is required")
	}

	res, err := s.db.Exec(
		`INSERT INTO leadDetails (lead_name, mobile, lead_type, created_at) VALUES (?, ?, ?, NOW())`,
		req.LeadName, req.Mobile, req.LeadType,
	)
	if err != nil {
		log.Printf("Failed to create lead: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to create lead: %v", err)
	}
	id, _ := res.LastInsertId()
	return s.getLeadByID(int32(id))
}

func (s *NotifyServiceServer) GetAllLeads(_ context.Context, _ *pb.CountsRequest) (*pb.LeadListResponse, error) {
	rows, err := s.db.Query(
		`SELECT lead_id, lead_name, mobile, lead_type, created_at FROM leadDetails ORDER BY created_at DESC`)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch leads: %v", err)
	}
	defer rows.Close()

	var list []*pb.Lead
	for rows.Next() {
		item, e := scanLead(rows.Scan)
		if e != nil {
			return nil, status.Errorf(codes.Internal, "scan error: %v", e)
		}
		list = append(list, item)
	}
	return &pb.LeadListResponse{Data: list}, rows.Err()
}

func (s *NotifyServiceServer) GetLeadByID(_ context.Context, req *pb.GetByLeadIDRequest) (*pb.LeadResponse, error) {
	if req.LeadId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: lead_id is required")
	}
	return s.getLeadByID(req.LeadId)
}

func (s *NotifyServiceServer) getLeadByID(leadID int32) (*pb.LeadResponse, error) {
	row := s.db.QueryRow(
		`SELECT lead_id, lead_name, mobile, lead_type, created_at FROM leadDetails WHERE lead_id = ?`, leadID)
	item, err := scanLead(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "Lead with lead_id %d not found", leadID)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch lead: %v", err)
	}
	return &pb.LeadResponse{Data: item}, nil
}

func scanLead(fn func(...any) error) (*pb.Lead, error) {
	var (
		m         pb.Lead
		createdAt sql.NullTime
	)
	if err := fn(&m.LeadId, &m.LeadName, &m.Mobile, &m.LeadType, &createdAt); err != nil {
		return nil, err
	}
	if createdAt.Valid {
		m.CreatedAt = fmtTime(createdAt.Time)
	}
	return &m, nil
}

// Booking RPCs

func (s *NotifyServiceServer) CreateBooking(_ context.Context, req *pb.CreateBookingRequest) (*pb.BookingResponse, error) {
	if req.LeadId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: lead_id is required")
	}
	if req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: lead_name is required")
	}
	if req.PaymentType == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: payment_type is required (TOKEN or BOOKING)")
	}
	if req.Amount <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: amount must be greater than 0")
	}
	if req.PaymentDate == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: payment_date is required (RFC3339)")
	}

	dt, err := parseTime(req.PaymentDate)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: payment_date must be RFC3339: %v", err)
	}

	// remaining_amount is 0 for full booking, equals amount for token (business rule)
	var remaining float64
	if req.PaymentType == "TOKEN" {
		remaining = req.Amount // token means full amount still remaining after token
	} else {
		remaining = 0 // full booking — nothing remaining
	}

	paymentStatus := req.PaymentStatus
	if paymentStatus == "" {
		paymentStatus = "PENDING"
	}

	res, err := s.db.Exec(
		`INSERT INTO booking (lead_id, payment_type, paid_amount, Remaining_amount, payment_date, payment_status, remarks)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		req.LeadId, req.PaymentType, req.Amount, remaining, dt, paymentStatus, req.Remarks,
	)
	if err != nil {
		log.Printf("Failed to create booking: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to create booking: %v", err)
	}
	id, _ := res.LastInsertId()
	return s.getBookingByID(int32(id))
}

func (s *NotifyServiceServer) GetAllBookings(_ context.Context, req *pb.GetBookingByLeadIDRequest) (*pb.BookingListResponse, error) {
	var rows *sql.Rows
	var err error

	const base = `SELECT b.booking_id, b.lead_id, l.lead_name, b.payment_type,
		b.paid_amount, b.Remaining_amount, b.payment_date,
		b.payment_status, b.remarks, b.created_at
		FROM booking b JOIN leadDetails l ON l.lead_id = b.lead_id`

	if req.LeadId > 0 {
		rows, err = s.db.Query(base+` WHERE b.lead_id = ? ORDER BY b.created_at DESC`, req.LeadId)
	} else {
		rows, err = s.db.Query(base + ` ORDER BY b.created_at DESC`)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch bookings: %v", err)
	}
	defer rows.Close()

	var list []*pb.Booking
	for rows.Next() {
		item, e := scanBooking(rows.Scan)
		if e != nil {
			return nil, status.Errorf(codes.Internal, "scan error: %v", e)
		}
		list = append(list, item)
	}
	return &pb.BookingListResponse{Data: list}, rows.Err()
}

func (s *NotifyServiceServer) GetBookingByID(_ context.Context, req *pb.GetBookingByIDRequest) (*pb.BookingResponse, error) {
	if req.BookingId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: booking_id is required")
	}
	return s.getBookingByID(req.BookingId)
}

func (s *NotifyServiceServer) getBookingByID(bookingID int32) (*pb.BookingResponse, error) {
	row := s.db.QueryRow(
		`SELECT b.booking_id, b.lead_id, l.lead_name, b.payment_type,
		        b.paid_amount, b.Remaining_amount, b.payment_date,
		        b.payment_status, b.remarks, b.created_at
		 FROM booking b JOIN leadDetails l ON l.lead_id = b.lead_id
		 WHERE b.booking_id = ?`, bookingID)
	item, err := scanBooking(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "Booking with booking_id %d not found", bookingID)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch booking: %v", err)
	}
	return &pb.BookingResponse{Data: item}, nil
}

func scanBooking(fn func(...any) error) (*pb.Booking, error) {
	var (
		m             pb.Booking
		remarks       sql.NullString
		paymentStatus sql.NullString
		paymentDate   *time.Time
		createdAt     *time.Time
	)
	if err := fn(
		&m.BookingId, &m.LeadId, &m.LeadName, &m.PaymentType,
		&m.PaidAmount, &m.RemainingAmount, &paymentDate,
		&paymentStatus, &remarks, &createdAt,
	); err != nil {
		return nil, err
	}
	m.PaymentStatus = paymentStatus.String
	m.Remarks = remarks.String
	if paymentDate != nil {
		m.PaymentDate = fmtTime(*paymentDate)
	}
	if createdAt != nil {
		m.CreatedAt = fmtTime(*createdAt)
	}
	return &m, nil
}

// Count RPC — returns total count of each entity

func (s *NotifyServiceServer) GetCounts(_ context.Context, _ *pb.CountsRequest) (*pb.CountsResponse, error) {
	var resp pb.CountsResponse

	queries := []struct {
		dest  *int32
		query string
	}{
		{&resp.TotalLeads, `SELECT COUNT(*) FROM leadDetails`},
		{&resp.TotalScheduled, `SELECT COUNT(*) FROM meeting_details WHERE milestone = 'SCHEDULED'`},
		{&resp.TotalRescheduled, `SELECT COUNT(*) FROM meeting_details WHERE milestone = 'RESCHEDULED'`},
		{&resp.TotalCancelled, `SELECT COUNT(*) FROM meeting_details WHERE milestone = 'CANCELLED'`},
		{&resp.TotalSuccess, `SELECT COUNT(*) FROM successful`},
		{&resp.TotalBookings, `SELECT COUNT(*) FROM booking`},
	}

	for _, q := range queries {
		if err := s.db.QueryRow(q.query).Scan(q.dest); err != nil {
			log.Printf("Failed to get count: %v", err)
			return nil, status.Errorf(codes.Internal, "Failed to get counts: %v", err)
		}
	}

	log.Printf("[GetCounts] leads=%d scheduled=%d rescheduled=%d cancelled=%d success=%d bookings=%d",
		resp.TotalLeads, resp.TotalScheduled, resp.TotalRescheduled,
		resp.TotalCancelled, resp.TotalSuccess, resp.TotalBookings)

	return &resp, nil
}
