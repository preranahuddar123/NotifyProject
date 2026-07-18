package notify

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// writeResponse marshals a proto message (or gRPC error) to the HTTP response.
func writeResponse(w http.ResponseWriter, msg proto.Message, err error) {
	if err != nil {
		st, _ := status.FromError(err)
		httpCode := runtime.HTTPStatusFromCode(st.Code())
		http.Error(w, st.Message(), httpCode)
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
