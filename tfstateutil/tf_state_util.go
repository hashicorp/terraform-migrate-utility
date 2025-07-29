package tfstateutil

import (
	"context"
	"fmt"
	"github.com/tidwall/gjson"
)

const (
	managedResourcesPath = `resources.#(mode=="managed")#`
	moduleKey            = `module`
)

type tfWorkspaceStateUtility struct {
	ctx context.Context
}

// TfWorkspaceStateUtility defines the interface for operations related to Terraform workspace state.
type TfWorkspaceStateUtility interface {
	GetManagedResources(workspaceStateData []byte) ([]byte, error)
	IsFullyModular(workspaceStateData []byte) bool
	WorkspaceToStackAddressMap(workspaceStateData []byte, stackSourceBundleAbsPath string) map[string]string
}

// NewTfWorkspaceStateUtility creates a new instance of TfWorkspaceStateUtility with the provided context.
// This utility is used to handle operations related to Terraform workspace state.
func NewTfWorkspaceStateUtility(ctx context.Context) TfWorkspaceStateUtility {
	return &tfWorkspaceStateUtility{
		ctx: ctx,
	}
}

// GetManagedResources retrieves all managed resources from the provided workspace state data.
// add an error handling when the workspace state data is not valid JSON or
// does not have any managed resources.
func (tf *tfWorkspaceStateUtility) GetManagedResources(workspaceStateData []byte) ([]byte, error) {
	if !gjson.Valid(string(workspaceStateData)) {
		return nil, fmt.Errorf("invalid workspace state data, expected valid JSON")
	}
	// use gjson to get all managed resources from the workspace state data
	// the path is defined as resources.#(mode=="managed")
	managedResources := gjson.GetBytes(workspaceStateData, managedResourcesPath)
	if !managedResources.Exists() {
		return nil, fmt.Errorf("no managed resources found in the workspace state data")
	}

	// return the array of managed resources
	return []byte(managedResources.Raw), nil
}

// IsFullyModular determines if all managed resources in the provided JSON array are associated with a module address.
// The input must be a valid JSON array of managed resources, typically obtained from GetManagedResources.
// Returns true if every resource has a module address, indicating the workspace is fully modular; otherwise, returns false.
func (tf *tfWorkspaceStateUtility) IsFullyModular(managedResourcesJson []byte) bool {
	managedResources := gjson.ParseBytes(managedResourcesJson).Array()
	missingModule := false

	// iterate through the resources to check if any of them are missing a module address
	// if any resource does not have a module address, the workspace is not fully modular
	for _, resource := range managedResources {
		// check if the resource has a module address
		if !resource.Get(moduleKey).Exists() {
			missingModule = true
			break
		}
	}

	return !missingModule
}

// WorkspaceToStackAddressMap converts the workspace state data to a map of workspace addresses to stack addresses.
func (tf *tfWorkspaceStateUtility) WorkspaceToStackAddressMap(workspaceStateData []byte, stackSourceBundleAbsPath string) map[string]string {
	panic("implement me")
}
