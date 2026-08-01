package notify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// writeResponse marshals a proto.Message to JSON and writes to the response.
// Used for all protoc-generated types.
func writeResponse(w http.ResponseWriter, msg proto.Message, err error) {
	if err != nil {
		st, _ := status.FromError(err)
		http.Error(w, st.Message(), runtime.HTTPStatusFromCode(st.Code()))
		return
	}
	codec := new(runtime.JSONPb)
	b, merr := codec.Marshal(msg)
	if merr != nil {
		http.Error(w, merr.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

// writeJSONResponse marshals any value to JSON and writes to the response.
// Used for hand-written types (Lead, Booking, Counts) that don't implement proto.Message.
func writeJSONResponse(w http.ResponseWriter, v interface{}, err error) {
	if err != nil {
		st, _ := status.FromError(err)
		http.Error(w, st.Message(), runtime.HTTPStatusFromCode(st.Code()))
		return
	}
	b, merr := json.Marshal(v)
	if merr != nil {
		http.Error(w, merr.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

// parseIntParam parses a path/query string value into *int32.
func parseIntParam(s string, dst *int32) error {
	if s == "" {
		return fmt.Errorf("empty param")
	}
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid integer param %q: %v", s, err)
	}
	if dst != nil {
		*dst = int32(v)
	}
	return nil
}
