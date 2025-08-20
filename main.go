package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-migrate-utility/rpcapi"
	"github.com/hashicorp/terraform-migrate-utility/rpcapi/terraform1/stacks"
	_ "github.com/hashicorp/terraform-migrate-utility/rpcapi/tfstackdata1"
	"github.com/hashicorp/terraform-migrate-utility/rpcapi/tfstacksagent1"
	stateOps "github.com/hashicorp/terraform-migrate-utility/stateops"
	stateUtil "github.com/hashicorp/terraform-migrate-utility/tfstateutil"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	//stackSourceBundleAbsPath  = "/Users/sujaysamanta/Workspace/Codebase/terraform-migrate-utility/terraform-no-mod-v1/modularized_config/_stacks_generated"
	//terraformStateFileAbsPath = "/Users/sujaysamanta/Workspace/Codebase/terraform-migrate-utility/terraform-no-mod-v1/modularized_config/terraform.tfstate"
	//terraformConfigDirAbsPath = "/Users/sujaysamanta/Workspace/Codebase/terraform-migrate-utility/terraform-no-mod-v1/modularized_config"

	stackSourceBundleAbsPath  = "/Users/sujaysamanta/Workspace/PlayGround/pet-nulls/_stacks_generated"
	terraformStateFileAbsPath = "/Users/sujaysamanta/Workspace/PlayGround/pet-nulls/terraform_state/terraform.tfstate"
	terraformConfigDirAbsPath = "/Users/sujaysamanta/Workspace/PlayGround/pet-nulls"
	fileWriteLocation         = "/Users/sujaysamanta/Workspace/PlayGround/pet-nulls/stack_state/"
)

func main() { // NOSONAR
	var err error
	ctx := context.Background()
	tfStateUtil := stateUtil.NewTfWorkspaceStateUtility(ctx)
	jsonOpts := protojson.MarshalOptions{
		Multiline: true,
	}

	// Read all resources from the Terraform state file
	resources, err := listAllResourcesFromWorkspaceStateExample(tfStateUtil, terraformConfigDirAbsPath)
	if err != nil {
		panic("Failed to list all resources from workspace state: " + err.Error())
	} else {
		fmt.Println("Resources in the Terraform state:")
		for _, resource := range resources {
			fmt.Println(resource)
		}
		fmt.Println()
	}

	// Check if the resources are fully modular
	stateFullyModular := isFullyModularExample(tfStateUtil, resources)
	if stateFullyModular {
		fmt.Println("The Terraform state is fully modular.")
		fmt.Println()
	} else {
		fmt.Println("The Terraform state is not fully modular.")
		fmt.Println()
	}

	// Create a map of workspace to stack address
	workspaceToStackMap, err := workspaceToStackAddressMapExample(tfStateUtil, terraformConfigDirAbsPath, stackSourceBundleAbsPath)
	if err != nil {
		panic("Failed to create workspace to stack address map: " + err.Error())
	} else {
		fmt.Println("Workspace to Stack Address Map:")
		indent, _ := json.MarshalIndent(workspaceToStackMap, "", "  ")
		fmt.Println(string(indent))
		fmt.Println()
	}

	// Raed raw Terraform state file
	rawTerraformState, err := readRawTerraformStateFile(terraformStateFileAbsPath)
	if err != nil {
		panic("Failed to read Terraform state file: " + err.Error())
	} else {
		fmt.Println("Raw terraform state read successfully.")
	}

	// read the current working directory
	currentWorkingDir, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting current working directory:", err)
	}

	// start the RPC client
	client, err := rpcapi.NewTerraformRpcClient(ctx)
	if err != nil {
		fmt.Println("Error connecting to RPC API:", err)
		return
	}

	stateOpsHandler := stateOps.NewTFStateOperations(ctx, client)

	stateConversionRequest := stateOps.WorkspaceToStackStateConversionRequest{
		Ctx:                         ctx,
		Client:                      client,
		CurrentWorkingDir:           currentWorkingDir,
		RawStateData:                rawTerraformState,
		StateOpsHandler:             stateOpsHandler,
		StackSourceBundleAbsPath:    stackSourceBundleAbsPath,
		TerraformConfigFilesAbsPath: terraformConfigDirAbsPath,
	}

	// If the state is fully modular, use the module address map, otherwise use the absolute resource address map
	if stateFullyModular {
		stateConversionRequest.ModuleAddressMap = workspaceToStackMap
	} else {
		stateConversionRequest.AbsoluteResourceAddressMap = workspaceToStackMap
	}

	stackState, err := convertTerraformWorkspaceToStackDataExample(stateConversionRequest)
	if err != nil {
		panic("Failed to convert Terraform workspace to stack data: " + err.Error())
	} else {
		fmt.Println(jsonOpts.Format(stackState))
		fmt.Println("Migration completed successfully!")
	}

	// Marshal to protobuf binary
	data, err := proto.Marshal(stackState)
	if err != nil {
		panic("Failed to marshal stack state to protobuf binary: " + err.Error())
	}

	// Write to a file
	if err := os.WriteFile(filepath.Join(fileWriteLocation, "stack_state.tfstackstate"), data, os.ModePerm); err != nil {
		panic("Failed to write stack state to file: " + err.Error())
	} else {
		fmt.Println("Stack state written to file successfully.")
	}

}

func readRawTerraformStateFile(stateFileName string) ([]byte, error) {
	return os.ReadFile(stateFileName)
}

func listAllResourcesFromWorkspaceStateExample(tfStateUtil stateUtil.TfWorkspaceStateUtility, workingDir string) ([]string, error) {
	return tfStateUtil.ListAllResourcesFromWorkspaceState(workingDir)
}

func isFullyModularExample(tfStateUtil stateUtil.TfWorkspaceStateUtility, resources []string) bool {
	return tfStateUtil.IsFullyModular(resources)
}

func workspaceToStackAddressMapExample(tfStateUtil stateUtil.TfWorkspaceStateUtility, terraformConfigFilesAbsPath string, stackSourceBundleAbsPath string) (map[string]string, error) {
	return tfStateUtil.WorkspaceToStackAddressMap(terraformConfigFilesAbsPath, stackSourceBundleAbsPath)
}

func convertTerraformWorkspaceToStackDataExample(stateConversionRequest stateOps.WorkspaceToStackStateConversionRequest) (*tfstacksagent1.StackState, error) { // NOSONAR
	// Ensure the RPC client is stopped after the conversion is done
	defer stateConversionRequest.Client.Stop()

	// Get the path for the stack modules cache directory
	stackModuleCacheDir := filepath.Join(stateConversionRequest.StackSourceBundleAbsPath, ".terraform/modules/")

	// Get the path for the Terraform provider cache directory
	terraformProviderCachePath := filepath.Join(stateConversionRequest.TerraformConfigFilesAbsPath, ".terraform/providers")

	// Get the relative path for the stack modules directory
	stackConfigRelativePath, err := filepath.Rel(stateConversionRequest.CurrentWorkingDir, stateConversionRequest.StackSourceBundleAbsPath)
	if err != nil {
		return nil, fmt.Errorf("error getting relative path for stack modules: %w", err)
	} else {
		if stackConfigRelativePath == "." {
			stackConfigRelativePath = "./"
		} else if !strings.HasPrefix(stackConfigRelativePath, "../") {
			stackConfigRelativePath = "./" + stackConfigRelativePath
		}
	}

	// Get the relative path for the Terraform dependency lock file
	terraformDependencyLockRelativePath, err := filepath.Rel(stateConversionRequest.CurrentWorkingDir, filepath.Join(stateConversionRequest.TerraformConfigFilesAbsPath, ".terraform.lock.hcl"))
	if err != nil {
		return nil, fmt.Errorf("error getting relative path for dependency lock file: %w", err)
	} else {
		if terraformDependencyLockRelativePath == "." {
			terraformDependencyLockRelativePath = "./"
		} else if !strings.HasPrefix(terraformDependencyLockRelativePath, "../") {
			terraformDependencyLockRelativePath = "./" + terraformDependencyLockRelativePath
		}
	}

	// Open raw Terraform state
	terraformStateHandle, closeTFState, err := stateConversionRequest.StateOpsHandler.OpenTerraformStateRaw(stateConversionRequest.RawStateData)
	if err != nil {
		return nil, fmt.Errorf("error opening Terraform state: %w", err)
	}
	defer closeTFState()

	// Open stack source bundle modules cache directory
	sourceBundleHandle, closeSourceBundle, err := stateConversionRequest.StateOpsHandler.OpenSourceBundle(stackModuleCacheDir)
	if err != nil {
		return nil, fmt.Errorf("error opening source bundle: %w", err)
	}
	defer closeSourceBundle()

	// Open stack configuration
	// This is the relative path from the current working directory to the stack configuration directory
	stackConfigHandle, closeStackConfig, err := stateConversionRequest.StateOpsHandler.OpenStacksConfiguration(sourceBundleHandle, stackConfigRelativePath)
	if err != nil {
		return nil, fmt.Errorf("error opening stack configuration: %w", err)
	}
	defer closeStackConfig()

	// Open the Terraform dependency lock file
	// This is the relative path from the current working directory to the lock file
	dependencyLocksHandle, closeDependencyLocks, err := stateConversionRequest.StateOpsHandler.OpenDependencyLockFile(sourceBundleHandle, terraformDependencyLockRelativePath)
	if err != nil {
		return nil, fmt.Errorf("error opening dependency lock file: %w", err)
	}
	defer closeDependencyLocks()

	// Open the Terraform provider cache directory
	providerCacheHandle, closeProviderCache, err := stateConversionRequest.StateOpsHandler.OpenProviderCache(terraformProviderCachePath)
	if err != nil {
		return nil, fmt.Errorf("error opening provider cache: %w", err)
	}
	defer closeProviderCache()

	// Perform the migration of the Terraform state to stack state
	// using the absolute resource address map or module address map
	if stateConversionRequest.AbsoluteResourceAddressMap == nil && stateConversionRequest.ModuleAddressMap == nil {
		return nil, fmt.Errorf("no resource or module address map provided for migration")
	}

	events, err := stateConversionRequest.StateOpsHandler.MigrateTFState(
		terraformStateHandle,
		stackConfigHandle,
		dependencyLocksHandle,
		providerCacheHandle,
		stateConversionRequest.AbsoluteResourceAddressMap,
		stateConversionRequest.ModuleAddressMap,
	)
	if err != nil {
		return nil, fmt.Errorf("error migrating Terraform state: %w", err)
	}

	stackState := &tfstacksagent1.StackState{
		FormatVersion: 1,
		Raw:           make(map[string]*anypb.Any),
		Descriptions:  make(map[string]*stacks.AppliedChange_ChangeDescription),
	}

	for {
		item, err := events.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("error receiving event: %w", err)
		}

		// Handle different event types
		switch result := item.Result.(type) {
		case *stacks.MigrateTerraformState_Event_AppliedChange:
			for _, raw := range result.AppliedChange.Raw {
				stackState.Raw[raw.Key] = raw.Value
			}

			for _, change := range result.AppliedChange.Descriptions {
				stackState.Descriptions[change.Key] = change
			}
		case *stacks.MigrateTerraformState_Event_Diagnostic:
			return nil, fmt.Errorf("diagnostic: %T", result.Diagnostic.Detail)
		default:
			return nil, fmt.Errorf("Received event: %T\n", result)
		}
	}

	return stackState, nil
}
