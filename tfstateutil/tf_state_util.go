package tfstateutil

import (
	"context"
)

type tfWorkspaceStateUtility struct {
	ctx context.Context
}

// TfWorkspaceStateUtility defines the interface for operations related to Terraform workspace state.
type TfWorkspaceStateUtility interface {
	IsFullyModular(workspaceStateData []byte) (bool, error)
	WorkspaceToStackAddressMap(workspaceStateData []byte, stackSourceBundleAbsPath string) map[string]string
}

// NewTfWorkspaceStateUtility creates a new instance of TfWorkspaceStateUtility with the provided context.
// This utility is used to handle operations related to Terraform workspace state.
func NewTfWorkspaceStateUtility(ctx context.Context) TfWorkspaceStateUtility {
	return &tfWorkspaceStateUtility{
		ctx: ctx,
	}
}

// IsFullyModular checks if the provided workspace state data is fully modular.
func (tf *tfWorkspaceStateUtility) IsFullyModular(workspaceStateData []byte) (bool, error) {
	panic("implement me")
}

// WorkspaceToStackAddressMap converts the workspace state data to a map of workspace addresses to stack addresses.
func (tf *tfWorkspaceStateUtility) WorkspaceToStackAddressMap(workspaceStateData []byte, stackSourceBundleAbsPath string) map[string]string {
	panic("implement me")
}
