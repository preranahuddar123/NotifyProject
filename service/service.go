package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"NotifyProject/internal/inbox"
	designpb "NotifyProject/proto/protogen/design"
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

	// Ensure the lead row exists so foreign key / joins work.
	// Booking requests don't carry lead_type, so we pass '' and only upsert the name.
	if _, err := s.db.Exec(
		`INSERT INTO leadDetails (lead_identifier, lead_name, lead_type, assigned_to, created_at)
		 VALUES (?, ?, '', ?, NOW())
		 ON DUPLICATE KEY UPDATE lead_name = VALUES(lead_name)`,
		req.LeadIdentifier, req.LeadName, req.AssignedTo,
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
		assignedTo    sql.NullString
		paymentType   sql.NullString
		remarks       sql.NullString
		paymentStatus sql.NullString
		paymentDate   sql.NullTime
		createdAt     sql.NullTime
	)
	if err := fn(
		&m.BookingId, &m.LeadIdentifier, &leadName, &assignedTo, &paymentType,
		&m.PaidAmount, &m.RemainingAmount, &paymentDate,
		&paymentStatus, &remarks, &createdAt,
	); err != nil {
		return nil, err
	}
	m.LeadName = leadName.String
	m.AssignedTo = assignedTo.String
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


// -----------------------------------------------------------------------
// DesignServiceServer Implementation (Handles the actual API requests)
// -----------------------------------------------------------------------

type DesignServiceServer struct {
	designpb.UnimplementedDesignServiceServer
	db *sql.DB
}

func NewDesignServiceServer(db *sql.DB) *DesignServiceServer {
	return &DesignServiceServer{db: db}
}

// Helper method to persist design notification in database.
func (s *DesignServiceServer) saveDesignNotification(
	eventID string,
	projectID string,
	leadName string,
	designerID int32,
	notifType string,
	notifAction string,
	payload any,
	recipients []*designpb.Recipient,
) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	if len(recipients) == 0 {
		recipients = []*designpb.Recipient{
			{UserId: designerID, Role: "designer"},
		}
	}

	var leadID int32
	projectIDStr := strings.TrimPrefix(projectID, "HUB-")
	if idVal, parseErr := strconv.Atoi(projectIDStr); parseErr == nil {
		leadID = int32(idVal)
	}

	var userIDs []int32
	for _, rec := range recipients {
		if rec.UserId <= 0 {
			continue
		}
		_, err = s.db.Exec(
			`INSERT INTO design_user_notifications 
			 (event_id, user_id, recipient_role, lead_id, project_id, lead_name, designer_id, 
			  notification_type, notification_action, payload, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
			 ON DUPLICATE KEY UPDATE event_id = event_id`,
			eventID,
			rec.UserId,
			rec.Role,
			leadID,
			projectID,
			leadName,
			designerID,
			notifType,
			notifAction,
			string(payloadBytes),
		)
		if err != nil {
			log.Printf("[service] Failed to save design notification for user %d: %v", rec.UserId, err)
			continue
		}
		userIDs = append(userIDs, rec.UserId)
	}

	if len(userIDs) > 0 {
		inbox.DefaultHub.Broadcast(userIDs, map[string]any{
			"type":     "inbox_updated",
			"event_id": eventID,
		})
	}

	log.Printf("[service] Saved fan-out design notification: project=%s type=%s action=%s recipients=%d", projectID, notifType, notifAction, len(userIDs))
	return nil
}

// API 01 — Pre-10% New Lead
func (s *DesignServiceServer) CreateDesignLeadPre10(
	_ context.Context,
	req *designpb.DesignLeadPre10Request,
) (*designpb.DesignLeadPre10Response, error) {

	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}

	var respPayload *designpb.DesignLeadPre10Response_Payload
	if req.Payload != nil {
		respPayload = &designpb.DesignLeadPre10Response_Payload{
			CurrentPhase:       req.Payload.CurrentPhase,
			DesignerName:       req.Payload.DesignerName,
			SalesExecutiveName: req.Payload.SalesExecutiveName,
			MeetingType:        req.Payload.MeetingType,
		}
		if req.Payload.Slot != nil {
			respPayload.Slot = &designpb.DesignLeadPre10Response_Payload_Slot{
				Date:     req.Payload.Slot.Date,
				SlotTime: req.Payload.Slot.SlotTime,
			}
		}
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.DesignerId, "LEAD", "CREATED", respPayload, req.Recipients)

	return &designpb.DesignLeadPre10Response{
		ProjectId:          req.ProjectId,
		LeadName:           req.LeadName,
		DesignerId:         req.DesignerId,
		NotificationType:   "LEAD",
		NotificationAction: "CREATED",
		Payload:            respPayload,
		CreatedAt:          time.Now().Format(time.RFC3339),
	}, nil
}

// API 02 — 10–20% Phase Entered
func (s *DesignServiceServer) CreateDesignLead1020(
	_ context.Context,
	req *designpb.DesignLead1020Request,
) (*designpb.DesignLead1020Response, error) {

	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}

	var payload *designpb.DesignLead1020Response_Data_Payload
	if req.Payload != nil {
		payload = &designpb.DesignLead1020Response_Data_Payload{
			PreviousPhase: req.Payload.PreviousPhase,
			Trigger:       req.Payload.Trigger,
			Message:       req.Payload.Message,
		}
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.DesignerId, "PHASE", "PHASE_ENTERED", payload, req.Recipients)

	return &designpb.DesignLead1020Response{
		Data: &designpb.DesignLead1020Response_Data{
			ProjectId:          req.ProjectId,
			LeadName:           req.LeadName,
			DesignerId:         req.DesignerId,
			NotificationType:   "PHASE",
			NotificationAction: "PHASE_ENTERED",
			Phase:              "PHASE_10_20",
			Payload:            payload,
			CreatedAt:          time.Now().Format(time.RFC3339),
		},
	}, nil
}

// API 03 — Milestone Completed
func (s *DesignServiceServer) CreateDesignMilestone(
	_ context.Context,
	req *designpb.DesignMilestoneRequest,
) (*designpb.DesignMilestoneResponse, error) {

	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}
	if req.Payload == nil {
		return nil, status.Errorf(codes.InvalidArgument, "payload is required")
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.DesignerId, "MILESTONE", "COMPLETED", req.Payload, req.Recipients)

	return &designpb.DesignMilestoneResponse{
		Data: &designpb.DesignMilestoneResponse_Data{
			ProjectId:          req.ProjectId,
			LeadName:           req.LeadName,
			DesignerId:         req.DesignerId,
			NotificationType:   "MILESTONE",
			NotificationAction: "COMPLETED",
			MilestoneName:      req.Payload.MilestoneName,
			TaskName:           req.Payload.TaskName,
			MilestoneIndex:     req.Payload.MilestoneIndex,
			DesignerName:       req.Payload.DesignerName,
			CreatedAt:          time.Now().Format(time.RFC3339),
		},
	}, nil
}

// API 04 — Payment Requested
func (s *DesignServiceServer) CreateDesignPaymentRequest(
	_ context.Context,
	req *designpb.DesignPaymentRequestRequest,
) (*designpb.DesignPaymentRequestResponse, error) {

	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}
	if req.Payload == nil {
		return nil, status.Errorf(codes.InvalidArgument, "payload is required")
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.DesignerId, "PAYMENT", "REQUESTED", req.Payload, req.Recipients)

	return &designpb.DesignPaymentRequestResponse{
		Data: &designpb.DesignPaymentRequestResponse_Data{
			ProjectId:          req.ProjectId,
			LeadName:           req.LeadName,
			DesignerId:         req.DesignerId,
			NotificationType:   "PAYMENT",
			NotificationAction: "REQUESTED",
			PaymentType:        designpb.PaymentType(req.Payload.PaymentType),
			UploadName:         req.Payload.UploadName,
			Amount:             req.Payload.Amount,
			CreatedAt:          time.Now().Format(time.RFC3339),
		},
	}, nil
}

// API 05 — Payment / Sales Closure Status
func (s *DesignServiceServer) CreateDesignPaymentStatus(
	_ context.Context,
	req *designpb.DesignPaymentStatusRequest,
) (*designpb.DesignPaymentStatusResponse, error) {
	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}

	respPayload := &designpb.DesignPaymentStatusResponse_Payload{
		Status:           req.Status,
		DecisionType:     req.DecisionType,
		PaymentType:      req.PaymentType,
		MilestoneContext: req.MilestoneContext,
		ApproverName:     req.ApproverName,
		Amount:           req.Amount,
		RejectionReason:  req.RejectionReason,
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.DesignerId, req.NotificationType, req.NotificationAction, respPayload, req.Recipients)

	return &designpb.DesignPaymentStatusResponse{
		ProjectId:  req.ProjectId,
		LeadName:   req.LeadName,
		DesignerId: req.DesignerId,
		Payload:    respPayload,
	}, nil
}

// API 06 — DQC Requested
func (s *DesignServiceServer) CreateDesignDQCRequest(
	_ context.Context,
	req *designpb.DesignDQCRequestRequest,
) (*designpb.DesignDQCRequestResponse, error) {
	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}

	payload := map[string]any{
		"dqc_round":     req.DqcRound,
		"review_id":     req.ReviewId,
		"designer_name": req.DesignerName,
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.DesignerId, "DQC", "REQUESTED", payload, req.Recipients)

	return &designpb.DesignDQCRequestResponse{
		Data: &designpb.DesignDQCRequestResponse_Data{
			ProjectId:          req.ProjectId,
			LeadName:           req.LeadName,
			DesignerId:         req.DesignerId,
			NotificationType:   "DQC",
			NotificationAction: "REQUESTED",
			DqcRound:           req.DqcRound,
			ReviewId:           req.ReviewId,
			DesignerName:       req.DesignerName,
			CreatedAt:          time.Now().Format(time.RFC3339),
		},
	}, nil
}

// API 07 — DQC Status
func (s *DesignServiceServer) CreateDesignDQCStatus(
	_ context.Context,
	req *designpb.DesignDQCStatusRequest,
) (*designpb.DesignDQCStatusResponse, error) {
	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}

	respPayload := &designpb.DesignDQCStatusResponse_Payload{
		Status:          req.Status,
		DecisionType:    req.DecisionType,
		DqcRound:        req.DqcRound,
		DesignerName:    req.DesignerName,
		RejectionReason: req.RejectionReason,
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.DesignerId, req.NotificationType, req.NotificationAction, respPayload, req.Recipients)

	return &designpb.DesignDQCStatusResponse{
		ProjectId:  req.ProjectId,
		LeadName:   req.LeadName,
		DesignerId: req.DesignerId,
		Payload:    respPayload,
	}, nil
}

// API 08 — MMT Visit Requested
func (s *DesignServiceServer) CreateDesignMMTRequest(
	_ context.Context,
	req *designpb.DesignMMTRequestRequest,
) (*designpb.DesignMMTRequestResponse, error) {
	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}

	respPayload := &designpb.DesignMMTRequestResponse_Payload{
		MmtScope:       req.MmtScope,
		VisitDate:      req.VisitDate,
		VisitTime:      req.VisitTime,
		MmtManagerId:   req.MmtManagerId,
		DesignerName:   req.DesignerName,
		MmtManagerName: req.MmtManagerName,
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.DesignerId, req.NotificationType, req.NotificationAction, respPayload, req.Recipients)

	return &designpb.DesignMMTRequestResponse{
		ProjectId:  req.ProjectId,
		LeadName:   req.LeadName,
		DesignerId: req.DesignerId,
		Payload:    respPayload,
	}, nil
}

// API 09 — MMT Executive Assigned
func (s *DesignServiceServer) CreateDesignMMTAssign(
	_ context.Context,
	req *designpb.DesignMMTAssignRequest,
) (*designpb.DesignMMTAssignResponse, error) {
	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}
	if req.Payload == nil {
		return nil, status.Errorf(codes.InvalidArgument, "payload is required")
	}

	respPayload := &designpb.DesignMMTAssignResponse_Payload{
		AssignmentType: req.Payload.AssignmentType,
		ToName:         req.Payload.ToName,
		ToId:           req.Payload.ToId,
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.DesignerId, req.NotificationType, req.NotificationAction, respPayload, req.Recipients)

	return &designpb.DesignMMTAssignResponse{
		ProjectId:  req.ProjectId,
		LeadName:   req.LeadName,
		DesignerId: req.DesignerId,
		Payload:    respPayload,
	}, nil
}

// API 10 — MMT Documents Ready
func (s *DesignServiceServer) CreateDesignMMTDocReady(
	_ context.Context,
	req *designpb.DesignMMTDocReadyRequest,
) (*designpb.DesignMMTDocReadyResponse, error) {
	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}
	if req.Payload == nil {
		return nil, status.Errorf(codes.InvalidArgument, "payload is required")
	}

	respPayload := &designpb.DesignMMTDocReadyResponse_Payload{
		MmtScope:   req.Payload.MmtScope,
		Via:        req.Payload.Via,
		UploadName: req.Payload.UploadName,
		ApprovedBy: req.Payload.ApprovedBy,
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.DesignerId, req.NotificationType, req.NotificationAction, respPayload, req.Recipients)

	return &designpb.DesignMMTDocReadyResponse{
		ProjectId: req.ProjectId,
		LeadName:  req.LeadName,
		Payload:   respPayload,
	}, nil
}

// API 11 — Design Meeting Scheduled
func (s *DesignServiceServer) CreateDesignMeeting(
	_ context.Context,
	req *designpb.DesignMeetingRequest,
) (*designpb.DesignMeetingResponse, error) {
	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}

	respPayload := &designpb.DesignMeetingResponse_Payload{
		MeetingType: req.MeetingType,
		Mod:         req.Mod,
	}
	if req.Slot != nil {
		respPayload.Slot = &designpb.DesignMeetingResponse_Payload_Slot{
			Date:     req.Slot.Date,
			TimeSlot: req.Slot.TimeSlot,
		}
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.DesignerId, req.NotificationType, req.NotificationAction, respPayload, req.Recipients)

	return &designpb.DesignMeetingResponse{
		ProjectId: req.ProjectId,
		LeadName:  req.LeadName,
		Payload:   respPayload,
	}, nil
}

// API 12 — Designer Reassignment
func (s *DesignServiceServer) CreateDesignAssignDesigner(
	_ context.Context,
	req *designpb.DesignAssignDesignerRequest,
) (*designpb.DesignAssignDesignerResponse, error) {
	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}
	if req.Payload == nil {
		return nil, status.Errorf(codes.InvalidArgument, "payload is required")
	}

	respPayload := &designpb.DesignAssignDesignerResponse_Payload{
		AssignmentType: req.Payload.AssignmentType,
		FromId:         req.Payload.FromId,
		ToId:           req.Payload.ToId,
		FromName:       req.Payload.FromName,
		ToName:         req.Payload.ToName,
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.Payload.ToId, req.NotificationType, req.NotificationAction, respPayload, req.Recipients)

	return &designpb.DesignAssignDesignerResponse{
		ProjectId: req.ProjectId,
		LeadName:  req.LeadName,
		Payload:   respPayload,
	}, nil
}

// API 13 — PM Assignment
func (s *DesignServiceServer) CreateDesignAssignPM(
	_ context.Context,
	req *designpb.DesignAssignPMRequest,
) (*designpb.DesignAssignPMResponse, error) {
	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}
	if req.Payload == nil {
		return nil, status.Errorf(codes.InvalidArgument, "payload is required")
	}

	respPayload := &designpb.DesignAssignPMResponse_Data_Payload{
		AssignmentType: req.Payload.AssignmentType,
		ToId:           req.Payload.ToId,
		ToName:         req.Payload.ToName,
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.DesignerId, req.NotificationType, req.NotificationAction, respPayload, req.Recipients)

	return &designpb.DesignAssignPMResponse{
		Data: &designpb.DesignAssignPMResponse_Data{
			ProjectId:          req.ProjectId,
			LeadName:           req.LeadName,
			DesignerId:         req.DesignerId,
			NotificationType:   req.NotificationType,
			NotificationAction: req.NotificationAction,
			Payload:            respPayload,
			CreatedAt:          time.Now().Format(time.RFC3339),
		},
	}, nil
}

// API 14 — New Quote
func (s *DesignServiceServer) CreateDesignQuote(
	_ context.Context,
	req *designpb.DesignQuoteRequest,
) (*designpb.DesignQuoteResponse, error) {
	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}
	if req.Payload == nil {
		return nil, status.Errorf(codes.InvalidArgument, "payload is required")
	}

	respPayload := &designpb.DesignQuoteResponse_Payload{
		QuoteId:   req.Payload.QuoteId,
		QuoteLink: req.Payload.QuoteLink,
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.DesignerId, req.NotificationType, req.NotificationAction, respPayload, req.Recipients)

	return &designpb.DesignQuoteResponse{
		ProjectId: req.ProjectId,
		LeadName:  req.LeadName,
		Payload:   respPayload,
	}, nil
}

// API 15 — P2P Completed
func (s *DesignServiceServer) CreateDesignP2P(
	_ context.Context,
	req *designpb.DesignP2PRequest,
) (*designpb.DesignP2PResponse, error) {
	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}

	respPayload := &designpb.DesignP2PResponse_Payload{
		DesignerName: req.DesignerName,
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.DesignerId, req.NotificationType, req.NotificationAction, respPayload, req.Recipients)

	return &designpb.DesignP2PResponse{
		ProjectId:  req.ProjectId,
		LeadName:   req.LeadName,
		DesignerId: req.DesignerId,
		Payload:    respPayload,
	}, nil
}

// API 16 — PM Approve / Reject (after DQC2)
func (s *DesignServiceServer) CreateDesignPMStatus(
	_ context.Context,
	req *designpb.DesignPMStatusRequest,
) (*designpb.DesignPMStatusResponse, error) {
	if req.ProjectId == "" || req.LeadName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project_id and lead_name are required")
	}

	respPayload := &designpb.DesignPMStatusResponse_Payload{
		Status:          req.Status,
		DecisionType:    req.DecisionType,
		DqcRound:        req.DqcRound,
		DesignerName:    req.DesignerName,
		ApproverName:    req.ApproverName,
		RejectionReason: req.RejectionReason,
	}

	_ = s.saveDesignNotification(req.EventId, req.ProjectId, req.LeadName, req.DesignerId, req.NotificationType, req.NotificationAction, respPayload, req.Recipients)

	return &designpb.DesignPMStatusResponse{
		ProjectId:  req.ProjectId,
		LeadName:   req.LeadName,
		DesignerId: req.DesignerId,
		Payload:    respPayload,
	}, nil
}

// API 17 — Notification Feed
func (s *DesignServiceServer) GetDesignNotificationFeed(
	ctx context.Context,
	req *designpb.DesignNotificationFeedRequest,
) (*designpb.DesignNotificationFeedResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT project_id, lead_name, notification_type, notification_action, designer_id, payload, created_at 
	          FROM design_user_notifications`
	var args []any
	var whereClauses []string

	if req.ProjectId != "" {
		whereClauses = append(whereClauses, "project_id = ?")
		args = append(args, req.ProjectId)
	}
	if req.Since != "" {
		whereClauses = append(whereClauses, "created_at > ?")
		args = append(args, req.Since)
	}

	if len(whereClauses) > 0 {
		query += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to query feed: %v", err)
	}
	defer rows.Close()

	var data []*designpb.DesignNotificationFeedResponse_Data
	for rows.Next() {
		var item designpb.DesignNotificationFeedResponse_Data
		var payloadStr string
		var createdAt time.Time

		err := rows.Scan(
			&item.ProjectId,
			&item.LeadName,
			&item.NotificationType,
			&item.NotificationAction,
			&item.DesignerId,
			&payloadStr,
			&createdAt,
		)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan row: %v", err)
		}
		item.Payload = payloadStr
		item.CreatedAt = createdAt.Format(time.RFC3339)
		data = append(data, &item)
	}

	return &designpb.DesignNotificationFeedResponse{
		Data: data,
	}, nil
}

// API 18 — Notification Counts
func (s *DesignServiceServer) GetDesignNotificationCounts(
	ctx context.Context,
	req *designpb.DesignNotificationCountsRequest,
) (*designpb.DesignNotificationCountsResponse, error) {
	query := `SELECT notification_type, COUNT(*) FROM design_user_notifications`
	var args []any
	if req.Since != "" {
		query += " WHERE created_at > ?"
		args = append(args, req.Since)
	}
	query += " GROUP BY notification_type"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to query counts: %v", err)
	}
	defer rows.Close()

	var total int32
	byType := &designpb.DesignNotificationCountsResponse_ByType{}

	for rows.Next() {
		var notifType string
		var count int32
		if err := rows.Scan(&notifType, &count); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan count row: %v", err)
		}
		total += count
		switch notifType {
		case "LEAD":
			byType.LEAD = count
		case "PHASE":
			byType.PHASE = count
		case "MILESTONE":
			byType.MILESTONE = count
		case "PAYMENT":
			byType.PAYMENT = count
		case "DQC":
			byType.DQC = count
		case "MMT":
			byType.MMT = count
		case "MEETING":
			byType.MEETING = count
		case "ASSIGNMENT":
			byType.ASSIGNMENT = count
		case "QUOTE", "QUOTATION":
			byType.QUOTE = count
		case "P2P":
			byType.P2P = count
		}
	}

	return &designpb.DesignNotificationCountsResponse{
		Data: &designpb.DesignNotificationCountsResponse_Data{
			Total:  total,
			ByType: byType,
		},
	}, nil
}

// API 19 — Notification Details
func (s *DesignServiceServer) GetDesignNotificationDetails(
	ctx context.Context,
	req *designpb.DesignNotificationDetailsRequest,
) (*designpb.DesignNotificationDetailsResponse, error) {
	if req.NotificationId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "notification_id is required")
	}

	query := `SELECT project_id, lead_name, notification_type, notification_action, designer_id, payload, created_at 
	          FROM design_user_notifications WHERE id = ?`

	var data designpb.DesignNotificationDetailsResponse_Data
	var payloadStr string
	var createdAt time.Time

	err := s.db.QueryRowContext(ctx, query, req.NotificationId).Scan(
		&data.ProjectId,
		&data.LeadName,
		&data.NotificationType,
		&data.NotificationAction,
		&data.DesignerId,
		&payloadStr,
		&createdAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "notification not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to query details: %v", err)
	}

	data.Payload = payloadStr
	data.CreatedAt = createdAt.Format(time.RFC3339)

	return &designpb.DesignNotificationDetailsResponse{
		Data: &data,
	}, nil
}

// GetDesignInbox queries the user-scoped inbox with filter parameters.
func (s *DesignServiceServer) GetDesignInbox(
	ctx context.Context,
	req *designpb.DesignInboxRequest,
) (*designpb.DesignInboxResponse, error) {
	store := inbox.NewStore(s.db)
	var since *time.Time
	if req.Since != "" {
		t, err := time.Parse(time.RFC3339, req.Since)
		if err == nil {
			since = &t
		}
	}
	rows, err := store.List(req.UserId, since, req.ProjectId, int(req.Limit))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to query inbox list: %v", err)
	}

	data := make([]*designpb.DesignInboxItem, len(rows))
	for i, r := range rows {
		var readAt string
		if r.ReadAt != nil {
			readAt = r.ReadAt.Format(time.RFC3339)
		}
		data[i] = &designpb.DesignInboxItem{
			Id:                 r.ID,
			EventId:            r.EventID,
			UserId:             r.UserID,
			RecipientRole:      r.RecipientRole,
			ProjectId:          r.ProjectID,
			LeadName:           r.LeadName,
			DesignerId:         r.DesignerID,
			NotificationType:   r.NotificationType,
			NotificationAction: r.NotificationAction,
			Payload:            r.Payload,
			CreatedAt:          r.CreatedAt.Format(time.RFC3339),
			ReadAt:             readAt,
		}
	}

	return &designpb.DesignInboxResponse{Data: data}, nil
}

// GetDesignInboxCounts returns unread counts grouped by notification type.
func (s *DesignServiceServer) GetDesignInboxCounts(
	ctx context.Context,
	req *designpb.DesignInboxCountsRequest,
) (*designpb.DesignInboxCountsResponse, error) {
	store := inbox.NewStore(s.db)
	var since *time.Time
	if req.Since != "" {
		t, err := time.Parse(time.RFC3339, req.Since)
		if err == nil {
			since = &t
		}
	}
	total, byType, err := store.Counts(req.UserId, since)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to query inbox counts: %v", err)
	}

	return &designpb.DesignInboxCountsResponse{
		Data: &designpb.DesignInboxCountsResponse_Data{
			Total: int32(total),
			ByType: &designpb.DesignInboxCountsResponse_ByType{
				LEAD:       int32(byType["LEAD"]),
				PHASE:      int32(byType["PHASE"]),
				MILESTONE:  int32(byType["MILESTONE"]),
				PAYMENT:    int32(byType["PAYMENT"]),
				DQC:        int32(byType["DQC"]),
				MMT:        int32(byType["MMT"]),
				MEETING:    int32(byType["MEETING"]),
				ASSIGNMENT: int32(byType["ASSIGNMENT"]),
				QUOTE:      int32(byType["QUOTE"]),
				P2P:        int32(byType["P2P"]),
			},
		},
	}, nil
}

// MarkDesignNotificationRead marks a single notification read and triggers WS update.
func (s *DesignServiceServer) MarkDesignNotificationRead(
	ctx context.Context,
	req *designpb.MarkDesignNotificationReadRequest,
) (*designpb.MarkDesignNotificationReadResponse, error) {
	store := inbox.NewStore(s.db)
	success, err := store.MarkRead(req.Id, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to mark read: %v", err)
	}
	if success {
		inbox.DefaultHub.Broadcast([]int32{req.UserId}, map[string]any{
			"type":   "inbox_updated",
			"reason": "read",
		})
	}
	return &designpb.MarkDesignNotificationReadResponse{Success: success}, nil
}

// MarkAllDesignNotificationsRead marks all notifications read for the user and triggers WS update.
func (s *DesignServiceServer) MarkAllDesignNotificationsRead(
	ctx context.Context,
	req *designpb.MarkAllDesignNotificationsReadRequest,
) (*designpb.MarkAllDesignNotificationsReadResponse, error) {
	store := inbox.NewStore(s.db)
	count, err := store.MarkAllRead(req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to mark all read: %v", err)
	}
	if count > 0 {
		inbox.DefaultHub.Broadcast([]int32{req.UserId}, map[string]any{
			"type":   "inbox_updated",
			"reason": "read-all",
		})
	}
	return &designpb.MarkAllDesignNotificationsReadResponse{
		Success: true,
		Count:   int32(count),
	}, nil
}

// CreateDesignInboxTicket issues a temporary WebSocket authentication ticket.
func (s *DesignServiceServer) CreateDesignInboxTicket(
	ctx context.Context,
	req *designpb.CreateDesignInboxTicketRequest,
) (*designpb.CreateDesignInboxTicketResponse, error) {
	ticket, err := inbox.DefaultTicketStore.Issue(req.UserId, 5*time.Minute)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to issue ticket: %v", err)
	}
	return &designpb.CreateDesignInboxTicketResponse{
		Ticket:    ticket,
		ExpiresIn: 300,
	}, nil
}
