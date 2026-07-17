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

// Helpers

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

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

func fmtTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

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
	var (
		rows *sql.Rows
		err  error
	)

	if req.LeadId > 0 {
		rows, err = s.db.Query(
			`SELECT meeting_id, lead_id, lead_name, reason, stage, event_datetime, created_at
			 FROM meeting_details WHERE milestone = 'CANCELLED' AND lead_id = ?
			 ORDER BY created_at DESC`, req.LeadId)
	} else {
		rows, err = s.db.Query(
			`SELECT meeting_id, lead_id, lead_name, reason, stage, event_datetime, created_at
			 FROM meeting_details WHERE milestone = 'CANCELLED'
			 ORDER BY created_at DESC`)
	}
	if err != nil {
		log.Printf("Failed to fetch cancellations: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to fetch cancellations: %v", err)
	}
	defer rows.Close()

	var list []*pb.MeetingCancellation
	for rows.Next() {
		item, e := scanCancellation(rows.Scan)
		if e != nil {
			return nil, status.Errorf(codes.Internal, "Failed to scan cancellation: %v", e)
		}
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "Row iteration error: %v", err)
	}

	return &pb.CancellationListResponse{Data: list}, nil
}

func (s *NotifyServiceServer) GetCancellationByID(_ context.Context, req *pb.GetByMeetingIDRequest) (*pb.CancellationResponse, error) {
	if req.MeetingId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: meeting_id is required")
	}
	return s.getCancellationByID(req.MeetingId)
}

func (s *NotifyServiceServer) UpdateCancellation(_ context.Context, req *pb.UpdateCancellationRequest) (*pb.CancellationResponse, error) {
	if req.MeetingId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: meeting_id is required")
	}

	dt, err := parseTime(req.EventDatetime)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: %v", err)
	}

	res, err := s.db.Exec(
		`UPDATE meeting_details SET reason = ?, stage = ?, event_datetime = ? WHERE meeting_id = ? AND milestone = 'CANCELLED'`,
		req.Reason, stageDBValue(req.Stage), dt, req.MeetingId,
	)
	if err != nil {
		log.Printf("Failed to update cancellation: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to update cancellation: %v", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, status.Errorf(codes.NotFound, "Cancellation with meeting_id %d not found", req.MeetingId)
	}

	return s.getCancellationByID(req.MeetingId)
}

func (s *NotifyServiceServer) DeleteCancellation(_ context.Context, req *pb.DeleteByMeetingIDRequest) (*pb.DeleteResponse, error) {
	if req.MeetingId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: meeting_id is required")
	}

	res, err := s.db.Exec(
		`DELETE FROM meeting_details WHERE meeting_id = ? AND milestone = 'CANCELLED'`,
		req.MeetingId,
	)
	if err != nil {
		log.Printf("Failed to delete cancellation: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to delete cancellation: %v", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, status.Errorf(codes.NotFound, "Cancellation with meeting_id %d not found", req.MeetingId)
	}

	return &pb.DeleteResponse{Message: "Deleted successfully"}, nil
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
	var (
		rows *sql.Rows
		err  error
	)

	const baseQuery = `
		SELECT s.lead_id, l.lead_name, s.description, s.stage,
		       s.success_datetime, s.next_action, s.quote_link_generated, s.created_at
		FROM successful s
		JOIN leadDetails l ON l.lead_id = s.lead_id`

	if req.LeadId > 0 {
		rows, err = s.db.Query(baseQuery+` WHERE s.lead_id = ? ORDER BY s.created_at DESC`, req.LeadId)
	} else {
		rows, err = s.db.Query(baseQuery + ` ORDER BY s.created_at DESC`)
	}
	if err != nil {
		log.Printf("Failed to fetch success records: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to fetch success records: %v", err)
	}
	defer rows.Close()

	var list []*pb.MeetingSuccess
	for rows.Next() {
		item, e := scanSuccess(rows.Scan)
		if e != nil {
			return nil, status.Errorf(codes.Internal, "Failed to scan success record: %v", e)
		}
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "Row iteration error: %v", err)
	}

	return &pb.SuccessListResponse{Data: list}, nil
}

func (s *NotifyServiceServer) GetSuccessByLeadID(_ context.Context, req *pb.GetByLeadIDRequest) (*pb.SuccessResponse, error) {
	if req.LeadId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: lead_id is required")
	}
	return s.getSuccessByLeadID(req.LeadId)
}

func (s *NotifyServiceServer) UpdateSuccess(_ context.Context, req *pb.UpdateSuccessRequest) (*pb.SuccessResponse, error) {
	if req.LeadId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: lead_id is required")
	}

	dt, err := parseTime(req.SuccessDatetime)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: %v", err)
	}

	res, err := s.db.Exec(
		`UPDATE successful SET description = ?, stage = ?, success_datetime = ?, next_action = ?, quote_link_generated = ? WHERE lead_id = ?`,
		req.Description, stageDBValue(req.Stage), dt, req.NextAction, req.QuoteLinkGenerated, req.LeadId,
	)
	if err != nil {
		log.Printf("Failed to update success record: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to update success record: %v", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, status.Errorf(codes.NotFound, "Success record for lead_id %d not found", req.LeadId)
	}

	return s.getSuccessByLeadID(req.LeadId)
}

func (s *NotifyServiceServer) DeleteSuccess(_ context.Context, req *pb.GetByLeadIDRequest) (*pb.DeleteResponse, error) {
	if req.LeadId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: lead_id is required")
	}

	res, err := s.db.Exec(`DELETE FROM successful WHERE lead_id = ?`, req.LeadId)
	if err != nil {
		log.Printf("Failed to delete success record: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to delete success record: %v", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, status.Errorf(codes.NotFound, "Success record for lead_id %d not found", req.LeadId)
	}

	return &pb.DeleteResponse{Message: "Deleted successfully"}, nil
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
		log.Printf("Failed to fetch cancellation: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to fetch cancellation: %v", err)
	}
	return &pb.CancellationResponse{Data: item}, nil
}

func (s *NotifyServiceServer) getSuccessByLeadID(leadID int32) (*pb.SuccessResponse, error) {
	row := s.db.QueryRow(
		`SELECT s.lead_id, l.lead_name, s.description, s.stage,
		        s.success_datetime, s.next_action, s.quote_link_generated, s.created_at
		 FROM successful s
		 JOIN leadDetails l ON l.lead_id = s.lead_id
		 WHERE s.lead_id = ? LIMIT 1`, leadID)

	item, err := scanSuccess(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "Success record for lead_id %d not found", leadID)
	}
	if err != nil {
		log.Printf("Failed to fetch success record: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to fetch success record: %v", err)
	}
	return &pb.SuccessResponse{Data: item}, nil
}

// Row scanners

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
	if err := fn(
		&m.LeadId, &m.LeadName, &description, &stage,
		&successDt, &nextAction, &m.QuoteLinkGenerated, &createdAt,
	); err != nil {
		return nil, err
	}
	m.Description = description.String
	m.NextAction = nextAction.String
	m.Stage = stageProtoValue(stage)
	m.SuccessDatetime = fmtTime(successDt)
	m.CreatedAt = fmtTime(createdAt)
	return &m, nil
}
