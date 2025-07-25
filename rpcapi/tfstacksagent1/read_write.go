package tfstacksagent1

import (
	"fmt"
	"io"

	"terraform-migrate-utility/rpcapi/terraform1/stacks"

	"google.golang.org/protobuf/proto"
	anypb "google.golang.org/protobuf/types/known/anypb"
)

func ReadStateSnapshot(r io.Reader) (*StackState, error) {
	state, err := readProto[StackState, *StackState](r)

	// Protobuf does not distinguish between an empty map and a nil map, but our
	// code expects the map to always be present even if empty. Here we shim the
	// unmarshalled result to be consistent with the initialized value, so that
	// we don't have to nil-check elsewhere.
	if state != nil {
		if state.Raw == nil {
			state.Raw = make(map[string]*anypb.Any)
		}
		if state.Descriptions == nil {
			state.Descriptions = make(map[string]*stacks.AppliedChange_ChangeDescription)
		}
	}

	return state, err
}

func WriteStateSnapshot(w io.Writer, data *StackState) error {
	return writeProto(w, data)
}

func readProto[T any, PT interface {
	*T
	proto.Message
}](r io.Reader) (PT, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var ret PT = new(T)
	err = proto.Unmarshal(raw, ret)
	return ret, err
}

func writeProto[T proto.Message](w io.Writer, data T) error {
	raw, err := proto.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to serialize the plan: %w", err)
	}

	_, err = w.Write(raw)
	return err
}
