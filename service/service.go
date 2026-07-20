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

// -----------------------------------------------------------------------
// Cancellation RPCs
// -----------------------------------------------------------------------

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

// -----------------------------------------------------------------------
// Success RPCs
// -----------------------------------------------------------------------

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

// -----------------------------------------------------------------------
// Scheduled RPCs
// -----------------------------------------------------------------------

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

// -----------------------------------------------------------------------
// Rescheduled RPCs
// -----------------------------------------------------------------------

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

// -----------------------------------------------------------------------
// Private fetch helpers
// -----------------------------------------------------------------------

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

// -----------------------------------------------------------------------
// Row scanners
// -----------------------------------------------------------------------

func scanCancellation(fn func(...any) error) (*pb.MeetingCancellation, error) {
	var (
		m         pb.MeetingCancellation
		reason    sql.NullString
		stage     string
		eventTime time.Time
		createdAt time.Time
	)
	if err := fn(&m.MeetingId, &m.LeadId, &m.LeadName, &reason, &stage, &eventTime, &createdAt); err != nil {
		return nil, err
	}
	m.Reason = reason.String
	m.Stage = stageProtoValue(stage)
	m.EventDatetime = fmtTime(eventTime)
	m.CreatedAt = fmtTime(createdAt)
	return &m, nil
}

func scanSuccess(fn func(...any) error) (*pb.MeetingSuccess, error) {
	var (
		m           pb.MeetingSuccess
		description sql.NullString
		stage       string
		nextAction  sql.NullString
		successDt   time.Time
		createdAt   time.Time
	)
	if err := fn(&m.LeadId, &m.LeadName, &description, &stage, &successDt, &nextAction, &m.QuoteLinkGenerated, &createdAt); err != nil {
		return nil, err
	}
	m.Description = description.String
	m.NextAction = nextAction.String
	m.Stage = stageProtoValue(stage)
	m.SuccessDatetime = fmtTime(successDt)
	m.CreatedAt = fmtTime(createdAt)
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
