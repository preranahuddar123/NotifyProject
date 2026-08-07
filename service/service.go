package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

// milestoneDBValue converts proto MeetingMilestone to the DB enum string.
func milestoneDBValue(m pb.MeetingMilestone) string {
	switch m {
	case pb.MeetingMilestone_MILESTONE_CONNECTION:
		return "CONNECTION"
	case pb.MeetingMilestone_MILESTONE_EXPERIENCE_DESIGN:
		return "EXPERIENCE_AND_DESIGN"
	default:
		return ""
	}
}

// milestoneProtoValue converts DB enum string to proto MeetingMilestone.
func milestoneProtoValue(s string) pb.MeetingMilestone {
	switch s {
	case "CONNECTION":
		return pb.MeetingMilestone_MILESTONE_CONNECTION
	case "EXPERIENCE_AND_DESIGN":
		return pb.MeetingMilestone_MILESTONE_EXPERIENCE_DESIGN
	default:
		return pb.MeetingMilestone_MILESTONE_UNSPECIFIED
	}
}

func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339, s) }
func fmtTime(t time.Time) string            { return t.Format(time.RFC3339) }

// meetingTitle builds the auto-generated title: "{sub_stage} meeting with {lead_name}"
func meetingTitle(subStage, leadName string) string {
	return fmt.Sprintf("%s meeting with %s", subStage, leadName)
}

// -----------------------------------------------------------------------
// Cancellation RPCs
// -----------------------------------------------------------------------

func (s *NotifyServiceServer) Cancellation(_ context.Context, req *pb.CancellationRequest) (*pb.CancellationResponse, error) {
	if req.LeadIdentifier == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "lead_identifier and lead_name are required")
	}
	if req.Milestone == pb.MeetingMilestone_MILESTONE_UNSPECIFIED {
		return nil, status.Errorf(codes.InvalidArgument, "milestone must be MILESTONE_CONNECTION or MILESTONE_EXPERIENCE_DESIGN")
	}
	title := meetingTitle("CANCELLED", req.LeadName)
	res, err := s.db.Exec(
		`INSERT INTO meeting_details (lead_identifier, lead_name, assigned_to, sub_stage, title, milestone, created_at)
		 VALUES (?, ?, ?, 'CANCELLED', ?, ?, NOW())`,
		req.LeadIdentifier, req.LeadName, req.AssignedTo, title, milestoneDBValue(req.Milestone),
	)
	if err != nil {
		log.Printf("Failed to create cancellation: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to create cancellation: %v", err)
	}
	id, _ := res.LastInsertId()
	return s.getCancellationByID(int32(id))
}

func (s *NotifyServiceServer) GetAllCancellations(_ context.Context, req *pb.GetByLeadIdentifierRequest) (*pb.CancellationListResponse, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if req.LeadIdentifier != "" {
		rows, err = s.db.Query(
			`SELECT meeting_id, lead_identifier, lead_name,  assigned_to, milestone, created_at
			 FROM meeting_details WHERE sub_stage = 'CANCELLED' AND lead_identifier = ?
			 ORDER BY created_at DESC`, req.LeadIdentifier)
	} else {
		rows, err = s.db.Query(
			`SELECT meeting_id, lead_identifier, lead_name,  assigned_to, milestone, created_at
			 FROM meeting_details WHERE sub_stage = 'CANCELLED' ORDER BY created_at DESC`)
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
		return nil, status.Errorf(codes.InvalidArgument, "meeting_id is required")
	}
	return s.getCancellationByID(req.MeetingId)
}

// -----------------------------------------------------------------------
// Success RPCs
// -----------------------------------------------------------------------

func (s *NotifyServiceServer) CreateSuccess(_ context.Context, req *pb.CreateSuccessRequest) (*pb.SuccessResponse, error) {
	if req.LeadIdentifier == "" {
		return nil, status.Errorf(codes.InvalidArgument, "lead_identifier is required")
	}
	if req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "lead_name is required")
	}
	if req.NextAction == "" {
		return nil, status.Errorf(codes.InvalidArgument, "next_action is required")
	}
	_, err := s.db.Exec(
		`INSERT INTO successful (lead_identifier, lead_name, assigned_to, next_action, quote_link_generated, created_at) VALUES (?, ?, ?, ?, ?, NOW())`,
		req.LeadIdentifier, req.LeadName, req.AssignedTo, req.NextAction, req.QuoteLinkGenerated,
	)
	if err != nil {
		log.Printf("Failed to create success record: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to create success record: %v", err)
	}
	return s.getSuccessByLeadIdentifier(req.LeadIdentifier)
}

func (s *NotifyServiceServer) GetAllSuccesses(_ context.Context, req *pb.GetByLeadIdentifierRequest) (*pb.SuccessListResponse, error) {
	const base = `SELECT lead_identifier, lead_name, assigned_to, next_action, quote_link_generated, created_at FROM successful`
	var (
		rows *sql.Rows
		err  error
	)
	if req.LeadIdentifier != "" {
		rows, err = s.db.Query(base+` WHERE lead_identifier = ? ORDER BY created_at DESC`, req.LeadIdentifier)
	} else {
		rows, err = s.db.Query(base + ` ORDER BY created_at DESC`)
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

func (s *NotifyServiceServer) GetSuccessByLeadIdentifier(_ context.Context, req *pb.GetByLeadIdentifierRequest) (*pb.SuccessResponse, error) {
	if req.LeadIdentifier == "" {
		return nil, status.Errorf(codes.InvalidArgument, "lead_identifier is required")
	}
	return s.getSuccessByLeadIdentifier(req.LeadIdentifier)
}

// -----------------------------------------------------------------------
// Scheduled RPCs
// -----------------------------------------------------------------------

func (s *NotifyServiceServer) CreateScheduled(_ context.Context, req *pb.CreateScheduledRequest) (*pb.ScheduledResponse, error) {
	if req.LeadIdentifier == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "lead_identifier and lead_name are required")
	}
	if req.MeetingDate == "" {
		return nil, status.Errorf(codes.InvalidArgument, "meeting_date is required")
	}
	if req.Slot == "" {
		return nil, status.Errorf(codes.InvalidArgument, "slot is required")
	}
	if req.MeetingType == "" {
		return nil, status.Errorf(codes.InvalidArgument, "meeting_type is required (VIRTUAL_MEETING, SHOWROOM_VISIT, SITE_VISIT)")
	}
	if req.Milestone == pb.MeetingMilestone_MILESTONE_UNSPECIFIED {
		return nil, status.Errorf(codes.InvalidArgument, "milestone is required")
	}
	title := meetingTitle("SCHEDULED", req.LeadName)
	res, err := s.db.Exec(
		`INSERT INTO meeting_details (lead_identifier, lead_name,  assigned_to, sub_stage, title, meeting_date, slot, meeting_type, milestone, created_at)
		 VALUES (?, ?, ?, 'SCHEDULED', ?, ?, ?, ?, ?, NOW())`,
		req.LeadIdentifier, req.LeadName, req.AssignedTo, title,
		req.MeetingDate, req.Slot, req.MeetingType, milestoneDBValue(req.Milestone),
	)
	if err != nil {
		log.Printf("Failed to create scheduled meeting: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to create scheduled meeting: %v", err)
	}
	id, _ := res.LastInsertId()
	return s.getScheduledByID(int32(id))
}

func (s *NotifyServiceServer) GetAllScheduled(_ context.Context, req *pb.GetScheduledByLeadIdentifierRequest) (*pb.ScheduledListResponse, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if req.LeadIdentifier != "" {
		rows, err = s.db.Query(
			`SELECT meeting_id, lead_identifier, lead_name,  assigned_to, title, meeting_date, slot, meeting_type, milestone, created_at
			 FROM meeting_details WHERE sub_stage = 'SCHEDULED' AND lead_identifier = ?
			 ORDER BY created_at DESC`, req.LeadIdentifier)
	} else {
		rows, err = s.db.Query(
			`SELECT meeting_id, lead_identifier, lead_name,  assigned_to, title, meeting_date, slot, meeting_type, milestone, created_at
			 FROM meeting_details WHERE sub_stage = 'SCHEDULED' ORDER BY created_at DESC`)
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
		return nil, status.Errorf(codes.InvalidArgument, "meeting_id is required")
	}
	return s.getScheduledByID(req.MeetingId)
}

// -----------------------------------------------------------------------
// Rescheduled RPCs
// -----------------------------------------------------------------------

func (s *NotifyServiceServer) Rescheduled(_ context.Context, req *pb.RescheduledRequest) (*pb.RescheduledResponse, error) {
	if req.LeadIdentifier == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "lead_identifier and lead_name are required")
	}
	if req.MeetingDate == "" {
		return nil, status.Errorf(codes.InvalidArgument, "meeting_date is required")
	}
	if req.Slot == "" {
		return nil, status.Errorf(codes.InvalidArgument, "slot is required")
	}
	if req.MeetingType == "" {
		return nil, status.Errorf(codes.InvalidArgument, "meeting_type is required (VIRTUAL_MEETING, SHOWROOM_VISIT, SITE_VISIT)")
	}
	if req.Milestone == pb.MeetingMilestone_MILESTONE_UNSPECIFIED {
		return nil, status.Errorf(codes.InvalidArgument, "milestone is required")
	}
	title := meetingTitle("RESCHEDULED", req.LeadName)
	res, err := s.db.Exec(
		`INSERT INTO meeting_details (lead_identifier, lead_name,  assigned_to, sub_stage, title, meeting_date, slot, meeting_type, milestone, created_at)
		 VALUES (?, ?, ?, 'RESCHEDULED', ?, ?, ?, ?, ?, NOW())`,
		req.LeadIdentifier, req.LeadName, req.AssignedTo, title,
		req.MeetingDate, req.Slot, req.MeetingType, milestoneDBValue(req.Milestone),
	)
	if err != nil {
		log.Printf("Failed to create rescheduled meeting: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to create rescheduled meeting: %v", err)
	}
	id, _ := res.LastInsertId()
	return s.getRescheduledByID(int32(id))
}

func (s *NotifyServiceServer) GetAllRescheduled(_ context.Context, req *pb.GetRescheduledByLeadIdentifierRequest) (*pb.RescheduledListResponse, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if req.LeadIdentifier != "" {
		rows, err = s.db.Query(
			`SELECT meeting_id, lead_identifier, lead_name,  assigned_to, title, meeting_date, slot, meeting_type, milestone, created_at
			 FROM meeting_details WHERE sub_stage = 'RESCHEDULED' AND lead_identifier = ?
			 ORDER BY created_at DESC`, req.LeadIdentifier)
	} else {
		rows, err = s.db.Query(
			`SELECT meeting_id, lead_identifier, lead_name, assigned_to, title, meeting_date, slot, meeting_type, milestone, created_at
			 FROM meeting_details WHERE sub_stage = 'RESCHEDULED' ORDER BY created_at DESC`)
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
		return nil, status.Errorf(codes.InvalidArgument, "meeting_id is required")
	}
	return s.getRescheduledByID(req.MeetingId)
}

// -----------------------------------------------------------------------
// Lead RPCs
// -----------------------------------------------------------------------

func (s *NotifyServiceServer) CreateLead(_ context.Context, req *pb.CreateLeadRequest) (*pb.LeadResponse, error) {
	if req.LeadIdentifier == "" {
		return nil, status.Errorf(codes.InvalidArgument, "lead_identifier is required")
	}
	if req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "lead_name is required")
	}
	if req.LeadType == "" {
		return nil, status.Errorf(codes.InvalidArgument, "lead_type is required")
	}
	_, err := s.db.Exec(
		`INSERT INTO leadDetails (lead_identifier, lead_name, lead_type, assigned_to, created_at) VALUES (?, ?, ?, ?, NOW())`,
		req.LeadIdentifier, req.LeadName, req.LeadType, req.AssignedTo,
	)
	if err != nil {
		log.Printf("Failed to create lead: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to create lead: %v", err)
	}
	return s.getLeadByIdentifier(req.LeadIdentifier)
}

func (s *NotifyServiceServer) GetAllLeads(_ context.Context, _ *pb.CountsRequest) (*pb.LeadListResponse, error) {
	rows, err := s.db.Query(
		`SELECT lead_identifier, lead_name, lead_type, assigned_to, created_at FROM leadDetails ORDER BY created_at DESC`)
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

func (s *NotifyServiceServer) GetLeadByIdentifier(_ context.Context, req *pb.GetByLeadIdentifierRequest) (*pb.LeadResponse, error) {
	if req.LeadIdentifier == "" {
		return nil, status.Errorf(codes.InvalidArgument, "lead_identifier is required")
	}
	return s.getLeadByIdentifier(req.LeadIdentifier)
}

// -----------------------------------------------------------------------
// Booking RPCs
// -----------------------------------------------------------------------

func (s *NotifyServiceServer) CreateBooking(_ context.Context, req *pb.CreateBookingRequest) (*pb.BookingResponse, error) {
	if req.LeadIdentifier == "" {
		return nil, status.Errorf(codes.InvalidArgument, "lead_identifier is required")
	}
	if req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "lead_name is required")
	}
	if req.PaymentType == "" {
		return nil, status.Errorf(codes.InvalidArgument, "payment_type is required (TOKEN or BOOKING)")
	}
	if req.Amount <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "amount must be greater than 0")
	}
	if req.PaymentDate == "" {
		return nil, status.Errorf(codes.InvalidArgument, "payment_date is required (RFC3339)")
	}
	dt, err := parseTime(req.PaymentDate)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "payment_date must be RFC3339: %v", err)
	}

	paymentType := req.PaymentType
	if paymentType == "BOOKING" || paymentType == "FULL_10%" || paymentType == "BOOKING FUll_10" {
		paymentType = "BOOKING FUll_10"
	}

	var remaining float64
	if paymentType == "TOKEN" {
		remaining = req.GetRemainingAmount()
	} else {
		remaining = 0.0
	}

	paymentStatus := req.PaymentStatus
	if paymentStatus == "" {
		paymentStatus = "PENDING"
	}

	// getBookingByID JOINs leadDetails — ensure the lead row exists first
	if _, err := s.db.Exec(
		`INSERT INTO leadDetails (lead_identifier, lead_name, lead_type, assigned_to, created_at)
		 VALUES (?, ?, 'ADD_LEAD', '', NOW())
		 ON DUPLICATE KEY UPDATE lead_name = VALUES(lead_name)`,
		req.LeadIdentifier, req.LeadName,
	); err != nil {
		log.Printf("Failed to upsert lead for booking: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to ensure lead exists: %v", err)
	}

	res, err := s.db.Exec(
		`INSERT INTO booking (lead_identifier, lead_name, assigned_to, payment_type, paid_amount, Remaining_amount, payment_date, payment_status, remarks, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())`,
		req.LeadIdentifier, req.LeadName, req.AssignedTo, paymentType, req.Amount, remaining, dt, paymentStatus, req.Remarks,
	)
	if err != nil {
		log.Printf("Failed to create booking: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to create booking: %v", err)
	}
	id, _ := res.LastInsertId()
	return s.getBookingByID(int32(id))
}

func (s *NotifyServiceServer) GetAllBookings(_ context.Context, req *pb.GetBookingByLeadIdentifierRequest) (*pb.BookingListResponse, error) {
	const base = `SELECT booking_id, lead_identifier, lead_name, assigned_to, payment_type,
		paid_amount, Remaining_amount, payment_date, payment_status, remarks, created_at
		FROM booking`
	var (
		rows *sql.Rows
		err  error
	)
	if req.LeadIdentifier != "" {
		rows, err = s.db.Query(base+` WHERE lead_identifier = ? ORDER BY created_at DESC`, req.LeadIdentifier)
	} else {
		rows, err = s.db.Query(base + ` ORDER BY created_at DESC`)
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
		return nil, status.Errorf(codes.InvalidArgument, "booking_id is required")
	}
	return s.getBookingByID(req.BookingId)
}

// -----------------------------------------------------------------------
// Counts RPC
// -----------------------------------------------------------------------

func (s *NotifyServiceServer) GetCounts(_ context.Context, _ *pb.CountsRequest) (*pb.CountsResponse, error) {
	var resp pb.CountsResponse
	queries := []struct {
		dest  *int32
		query string
	}{
		{&resp.TotalLeads, `SELECT COUNT(*) FROM leadDetails`},
		{&resp.TotalScheduled, `SELECT COUNT(*) FROM meeting_details WHERE sub_stage = 'SCHEDULED'`},
		{&resp.TotalRescheduled, `SELECT COUNT(*) FROM meeting_details WHERE sub_stage = 'RESCHEDULED'`},
		{&resp.TotalCancelled, `SELECT COUNT(*) FROM meeting_details WHERE sub_stage = 'CANCELLED'`},
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

// -----------------------------------------------------------------------
// Private fetch helpers
// -----------------------------------------------------------------------

func (s *NotifyServiceServer) getCancellationByID(meetingID int32) (*pb.CancellationResponse, error) {
	row := s.db.QueryRow(
		`SELECT meeting_id, lead_identifier, lead_name,  assigned_to, milestone, created_at
		 FROM meeting_details WHERE meeting_id = ? AND sub_stage = 'CANCELLED'`, meetingID)
	item, err := scanCancellation(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "Cancellation with meeting_id %d not found", meetingID)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch cancellation: %v", err)
	}
	return &pb.CancellationResponse{Data: item}, nil
}

func (s *NotifyServiceServer) getSuccessByLeadIdentifier(leadIdentifier string) (*pb.SuccessResponse, error) {
	row := s.db.QueryRow(
		`SELECT lead_identifier, lead_name,  assigned_to, next_action, quote_link_generated, created_at
		 FROM successful WHERE lead_identifier = ? ORDER BY created_at DESC LIMIT 1`, leadIdentifier)
	item, err := scanSuccess(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "Success record for lead_identifier %q not found", leadIdentifier)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch success record: %v", err)
	}
	return &pb.SuccessResponse{Data: item}, nil
}

func (s *NotifyServiceServer) getScheduledByID(id int32) (*pb.ScheduledResponse, error) {
	row := s.db.QueryRow(
		`SELECT meeting_id, lead_identifier, lead_name,  assigned_to, title, meeting_date, slot, meeting_type, milestone, created_at
		 FROM meeting_details WHERE meeting_id = ? AND sub_stage = 'SCHEDULED'`, id)
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
		`SELECT meeting_id, lead_identifier, lead_name,  assigned_to, title, meeting_date, slot, meeting_type, milestone, created_at
		 FROM meeting_details WHERE meeting_id = ? AND sub_stage = 'RESCHEDULED'`, id)
	item, err := scanRescheduled(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "Rescheduled meeting with meeting_id %d not found", id)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch rescheduled meeting: %v", err)
	}
	return &pb.RescheduledResponse{Data: item}, nil
}

func (s *NotifyServiceServer) getLeadByIdentifier(leadIdentifier string) (*pb.LeadResponse, error) {
	row := s.db.QueryRow(
		`SELECT lead_identifier, lead_name, lead_type, assigned_to, created_at
		 FROM leadDetails WHERE lead_identifier = ?`, leadIdentifier)
	item, err := scanLead(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "Lead with lead_identifier %q not found", leadIdentifier)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch lead: %v", err)
	}
	return &pb.LeadResponse{Data: item}, nil
}

func (s *NotifyServiceServer) getBookingByID(bookingID int32) (*pb.BookingResponse, error) {
	row := s.db.QueryRow(
		`SELECT booking_id, lead_identifier, lead_name,  assigned_to, payment_type,
		        paid_amount, Remaining_amount, payment_date, payment_status, remarks, created_at
		 FROM booking WHERE booking_id = ?`, bookingID)
	item, err := scanBooking(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "Booking with booking_id %d not found", bookingID)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to fetch booking: %v", err)
	}
	return &pb.BookingResponse{Data: item}, nil
}

// -----------------------------------------------------------------------
// Row scanners
// -----------------------------------------------------------------------

func scanCancellation(fn func(...any) error) (*pb.MeetingCancellation, error) {
	var (
		m         pb.MeetingCancellation
		milestone sql.NullString
		createdAt sql.NullTime
	)
	if err := fn(&m.MeetingId, &m.LeadIdentifier, &m.LeadName, &m.AssignedTo, &milestone, &createdAt); err != nil {
		return nil, err
	}
	m.Milestone = milestoneProtoValue(milestone.String)
	if createdAt.Valid {
		m.CreatedAt = fmtTime(createdAt.Time)
	}
	return &m, nil
}

func scanSuccess(fn func(...any) error) (*pb.MeetingSuccess, error) {
	var (
		m          pb.MeetingSuccess
		nextAction sql.NullString
		createdAt  sql.NullTime
	)
	if err := fn(&m.LeadIdentifier, &m.LeadName, &m.AssignedTo, &nextAction, &m.QuoteLinkGenerated, &createdAt); err != nil {
		return nil, err
	}
	m.NextAction = nextAction.String
	if createdAt.Valid {
		m.CreatedAt = fmtTime(createdAt.Time)
	}
	return &m, nil
}

func scanScheduled(fn func(...any) error) (*pb.MeetingScheduled, error) {
	var (
		m           pb.MeetingScheduled
		title       sql.NullString
		meetingDate sql.NullString
		slot        sql.NullString
		meetingType sql.NullString
		milestone   sql.NullString
		createdAt   sql.NullTime
	)
	if err := fn(&m.Id, &m.LeadIdentifier, &m.LeadName, &m.AssignedTo, &title, &meetingDate, &slot, &meetingType, &milestone, &createdAt); err != nil {
		return nil, err
	}
	m.Title = title.String
	m.MeetingDate = meetingDate.String
	m.Slot = slot.String
	m.MeetingType = meetingType.String
	m.Milestone = milestoneProtoValue(milestone.String)
	if createdAt.Valid {
		m.CreatedAt = fmtTime(createdAt.Time)
	}
	return &m, nil
}

func scanRescheduled(fn func(...any) error) (*pb.MeetingRescheduled, error) {
	var (
		m           pb.MeetingRescheduled
		title       sql.NullString
		meetingDate sql.NullString
		slot        sql.NullString
		meetingType sql.NullString
		milestone   sql.NullString
		createdAt   sql.NullTime
	)
	if err := fn(&m.Id, &m.LeadIdentifier, &m.LeadName, &m.AssignedTo, &title, &meetingDate, &slot, &meetingType, &milestone, &createdAt); err != nil {
		return nil, err
	}
	m.Title = title.String
	m.MeetingDate = meetingDate.String
	m.Slot = slot.String
	m.MeetingType = meetingType.String
	m.Milestone = milestoneProtoValue(milestone.String)
	if createdAt.Valid {
		m.CreatedAt = fmtTime(createdAt.Time)
	}
	return &m, nil
}

func scanLead(fn func(...any) error) (*pb.Lead, error) {
	var (
		m         pb.Lead
		createdAt sql.NullTime
	)
	if err := fn(&m.LeadIdentifier, &m.LeadName, &m.LeadType, &m.AssignedTo, &createdAt); err != nil {
		return nil, err
	}
	if createdAt.Valid {
		m.CreatedAt = fmtTime(createdAt.Time)
	}
	return &m, nil
}

func scanBooking(fn func(...any) error) (*pb.Booking, error) {
	var (
		m             pb.Booking
		leadName      sql.NullString
		paymentType   sql.NullString
		remarks       sql.NullString
		paymentStatus sql.NullString
		paymentDate   sql.NullTime
		createdAt     sql.NullTime
	)
	if err := fn(
		&m.BookingId, &m.LeadIdentifier, &m.LeadName, &m.AssignedTo, &m.PaymentType,
		&m.PaidAmount, &m.RemainingAmount, &paymentDate,
		&paymentStatus, &remarks, &createdAt,
	); err != nil {
		return nil, err
	}
	m.LeadName = leadName.String
	m.PaymentType = paymentType.String
	m.PaymentStatus = paymentStatus.String
	m.Remarks = remarks.String
	if paymentDate.Valid {
		m.PaymentDate = fmtTime(paymentDate.Time)
	}
	if createdAt.Valid {
		m.CreatedAt = fmtTime(createdAt.Time)
	}
	return &m, nil
}
