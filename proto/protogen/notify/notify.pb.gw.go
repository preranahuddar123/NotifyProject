// Hand-written gRPC-Gateway registration.
// Registers all active NotifyService RPCs as HTTP endpoints.

package notify

import (
	"context"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
)

func RegisterNotifyServiceHandlerFromEndpoint(ctx context.Context, mux *runtime.ServeMux, grpcEndpoint string, opts []grpc.DialOption) error {
	conn, err := grpc.NewClient(grpcEndpoint, opts...)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	return RegisterNotifyServiceHandler(ctx, mux, conn)
}

func RegisterNotifyServiceHandler(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error {
	client := NewNotifyServiceClient(conn)
	codec := new(runtime.JSONPb)

	// ---- Cancellation ----

	if err := mux.HandlePath("POST", "/v1/meetings/cancellation", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		var req CreateCancellationRequest
		if err := codec.NewDecoder(r.Body).Decode(&req); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		resp, err := client.CreateCancellation(ctx, &req)
		writeResponse(w, resp, err)
	}); err != nil { return err }

	if err := mux.HandlePath("GET", "/v1/meetings/cancellation", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		var req GetByLeadIDRequest
		if v := r.URL.Query().Get("lead_id"); v != "" { parseIntParam(v, &req.LeadId) }
		resp, err := client.GetAllCancellations(ctx, &req)
		writeResponse(w, resp, err)
	}); err != nil { return err }

	if err := mux.HandlePath("GET", "/v1/meetings/cancellation/{meeting_id}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		var req GetByMeetingIDRequest
		parseIntParam(params["meeting_id"], &req.MeetingId)
		resp, err := client.GetCancellationByID(ctx, &req)
		writeResponse(w, resp, err)
	}); err != nil { return err }

	// ---- Success ----

	if err := mux.HandlePath("POST", "/v1/meetings/success", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		var req CreateSuccessRequest
		if err := codec.NewDecoder(r.Body).Decode(&req); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		resp, err := client.CreateSuccess(ctx, &req)
		writeResponse(w, resp, err)
	}); err != nil { return err }

	if err := mux.HandlePath("GET", "/v1/meetings/success", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		var req GetByLeadIDRequest
		if v := r.URL.Query().Get("lead_id"); v != "" { parseIntParam(v, &req.LeadId) }
		resp, err := client.GetAllSuccesses(ctx, &req)
		writeResponse(w, resp, err)
	}); err != nil { return err }

	if err := mux.HandlePath("GET", "/v1/meetings/success/{lead_id}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		var req GetByLeadIDRequest
		parseIntParam(params["lead_id"], &req.LeadId)
		resp, err := client.GetSuccessByLeadID(ctx, &req)
		writeResponse(w, resp, err)
	}); err != nil { return err }

	// ---- Scheduled ----

	if err := mux.HandlePath("POST", "/v1/meetings/scheduled", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		var req CreateScheduledRequest
		if err := codec.NewDecoder(r.Body).Decode(&req); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		resp, err := client.CreateScheduled(ctx, &req)
		writeResponse(w, resp, err)
	}); err != nil { return err }

	if err := mux.HandlePath("GET", "/v1/meetings/scheduled", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		var req GetScheduledByLeadIDRequest
		if v := r.URL.Query().Get("lead_id"); v != "" { parseIntParam(v, &req.LeadId) }
		resp, err := client.GetAllScheduled(ctx, &req)
		writeResponse(w, resp, err)
	}); err != nil { return err }

	if err := mux.HandlePath("GET", "/v1/meetings/scheduled/{meeting_id}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		var req GetScheduledByIDRequest
		parseIntParam(params["meeting_id"], &req.MeetingId)
		resp, err := client.GetScheduledByID(ctx, &req)
		writeResponse(w, resp, err)
	}); err != nil { return err }

	// ---- Rescheduled ----

	if err := mux.HandlePath("POST", "/v1/meetings/rescheduled", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		var req CreateRescheduledRequest
		if err := codec.NewDecoder(r.Body).Decode(&req); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		resp, err := client.CreateRescheduled(ctx, &req)
		writeResponse(w, resp, err)
	}); err != nil { return err }

	if err := mux.HandlePath("GET", "/v1/meetings/rescheduled", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		var req GetRescheduledByLeadIDRequest
		if v := r.URL.Query().Get("lead_id"); v != "" { parseIntParam(v, &req.LeadId) }
		resp, err := client.GetAllRescheduled(ctx, &req)
		writeResponse(w, resp, err)
	}); err != nil { return err }

	if err := mux.HandlePath("GET", "/v1/meetings/rescheduled/{meeting_id}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		var req GetRescheduledByIDRequest
		parseIntParam(params["meeting_id"], &req.MeetingId)
		resp, err := client.GetRescheduledByID(ctx, &req)
		writeResponse(w, resp, err)
	}); err != nil { return err }

	// ---- Leads ----

	if err := mux.HandlePath("POST", "/v1/leads", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		var req CreateLeadRequest
		if err := codec.NewDecoder(r.Body).Decode(&req); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		resp, err := client.CreateLead(ctx, &req)
		writeJSONResponse(w, resp, err)
	}); err != nil { return err }

	if err := mux.HandlePath("GET", "/v1/leads", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		resp, err := client.GetAllLeads(ctx, &CountsRequest{})
		writeJSONResponse(w, resp, err)
	}); err != nil { return err }

	if err := mux.HandlePath("GET", "/v1/leads/{lead_id}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		var req GetByLeadIDRequest
		parseIntParam(params["lead_id"], &req.LeadId)
		resp, err := client.GetLeadByID(ctx, &req)
		writeJSONResponse(w, resp, err)
	}); err != nil { return err }

	// ---- Bookings ----

	if err := mux.HandlePath("POST", "/v1/bookings", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		var req CreateBookingRequest
		if err := codec.NewDecoder(r.Body).Decode(&req); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		resp, err := client.CreateBooking(ctx, &req)
		writeJSONResponse(w, resp, err)
	}); err != nil { return err }

	if err := mux.HandlePath("GET", "/v1/bookings", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		var req GetBookingByLeadIDRequest
		if v := r.URL.Query().Get("lead_id"); v != "" { parseIntParam(v, &req.LeadId) }
		resp, err := client.GetAllBookings(ctx, &req)
		writeJSONResponse(w, resp, err)
	}); err != nil { return err }

	if err := mux.HandlePath("GET", "/v1/bookings/{booking_id}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		var req GetBookingByIDRequest
		parseIntParam(params["booking_id"], &req.BookingId)
		resp, err := client.GetBookingByID(ctx, &req)
		writeJSONResponse(w, resp, err)
	}); err != nil { return err }

	// ---- Counts ----

	if err := mux.HandlePath("GET", "/v1/counts", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		resp, err := client.GetCounts(ctx, &CountsRequest{})
		writeJSONResponse(w, resp, err)
	}); err != nil { return err }

	return nil
}
