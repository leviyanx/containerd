package wasmdealer

import (
	"time"

	"github.com/gogo/protobuf/types"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Once we migrate off from gogo/protobuf, we can use the function below, which don't return any errors.
// convert "google.golang.org/protobuf/types/known/anypb".Any to "github.com/gogo/protobuf/types".Any
func anyFromPbToTypes(from *anypb.Any) *types.Any {
  return &types.Any{
    TypeUrl: from.TypeUrl,
    Value: from.Value,
  }
}

// ToTimestamp creates protobuf's Timestamp from time.Time.
func ToTimestamp(from time.Time) *timestamppb.Timestamp {
	return timestamppb.New(from)
}

// FromTimestamp creates time.Time from protobuf's Timestamp.
func FromTimestamp(from *timestamppb.Timestamp) time.Time {
	return from.AsTime()
}

