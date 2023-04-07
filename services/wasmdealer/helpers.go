package wasmdealer

import (
	"github.com/gogo/protobuf/types"
	"google.golang.org/protobuf/types/known/anypb"
)

// Once we migrate off from gogo/protobuf, we can use the function below, which don't return any errors.
// convert "google.golang.org/protobuf/types/known/anypb".Any to "github.com/gogo/protobuf/types".Any
func anyFromPbToTypes(from *anypb.Any) *types.Any {
  return &types.Any{
    TypeUrl: from.TypeUrl,
    Value: from.Value,
  }
}
