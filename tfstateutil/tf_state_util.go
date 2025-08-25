package tfstateutil

import (
	"context"
	"errors"
	"fmt"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	stackComponentHCLFileExt   = `.tfcomponent.hcl`
	firstLevelModuleExpression = `^module\.([^.]+)`
)

type tfWorkspaceStateUtility struct {
	ctx       context.Context
	hclParser *hclparse.Parser
}

func (t *tfWorkspaceStateUtility) UpdateContext(ctx context.Context) {
	t.ctx = ctx
}

// TfWorkspaceStateUtility defines the interface for utility functions related to Terraform workspace state.
type TfWorkspaceStateUtility interface {
	IsFullyModular(resources []string) bool
	ListAllResourcesFromWorkspaceState(workingDir string) ([]string, error)
	WorkspaceToStackAddressMap(terraformConfigFilesAbsPath string, stackSourceBundleAbsPath string) (map[string]string, error)
	UpdateContext(ctx context.Context)
}

// NewTfWorkspaceStateUtility creates a new instance of tfWorkspaceStateUtility with the provided context.
func NewTfWorkspaceStateUtility(ctx context.Context) TfWorkspaceStateUtility {
	return &tfWorkspaceStateUtility{
		ctx:       ctx,
		hclParser: hclparse.NewParser(),
	}
}

// IsFullyModular checks if all resource identifiers in the provided list have the prefix "module." Returns true if all are modular.
func (t *tfWorkspaceStateUtility) IsFullyModular(resources []string) bool {
	for _, resource := range resources {
		if !strings.HasPrefix(resource, "module.") {
			return false
		}
	}
	return true
}

// ListAllResourcesFromWorkspaceState lists all resources from the Terraform workspace state in the specified working directory.
// It executes the `terraform state list` command and returns the resources as a slice of strings
func (t *tfWorkspaceStateUtility) ListAllResourcesFromWorkspaceState(workingDir string) ([]string, error) {
	cmd := exec.CommandContext(t.ctx, "terraform", "state", "list")
	cmd.Dir = workingDir

	// Remove TF_LOG and TF_CLI_CONFIG_FILE from the environment for this command
	// (preserve all other environment variables)
	env := os.Environ()
	var filteredEnv []string
	for _, e := range env {
		if !strings.HasPrefix(e, "TF_LOG=") || !strings.HasPrefix(e, "TF_CLI_CONFIG_FILE=") {
			filteredEnv = append(filteredEnv, e)
		}
	}
	cmd.Env = filteredEnv

	// Capture only stdout
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("failed to run terraform state list: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to run terraform state list: %w", err)
	}

	// Convert to string and split by line
	resources := strings.Split(strings.TrimSpace(string(output)), "\n")

	if len(resources) == 0 {
		return nil, fmt.Errorf("no resources found in the Terraform state")
	}

	return resources, nil

}

// WorkspaceToStackAddressMap creates a mapping of workspace resources to stack addresses based on the provided Terraform configuration files and stack source bundle path.
// It validates the stack source bundle, retrieves stack files, extracts components, and maps resources to stack addresses.
// Returns a map where keys are resource identifiers and values are stack addresses.
// If the state is not fully modular, it expects exactly one component and maps all resources to that component's address.
// If the state is fully modular, it maps resources to their corresponding top-level module addresses.
func (t *tfWorkspaceStateUtility) WorkspaceToStackAddressMap(terraformConfigFilesAbsPath string, stackSourceBundleAbsPath string) (map[string]string, error) {
	var workspaceToStackAddressMap = make(map[string]string)

	//1. Validate the stack source bundle path
	//if _, err := t.validateStacksFiles(stackSourceBundleAbsPath); err != nil {
	//	return nil, fmt.Errorf("erro validating stack config files in path %s, err: %v", stackSourceBundleAbsPath, err)
	//}

	// 2. Get all stack files from the stack source bundle path
	stackFiles, err := t.getStackFiles(stackSourceBundleAbsPath)
	if err != nil {
		fmt.Printf("Error getting stack files: %v\n", err)
		return nil, err
	}

	// 3. Iterate through each stack file and extract components
	componentsSet := mapset.NewSet[string]()
	for _, filePath := range stackFiles {
		components, err := t.getAllComponents(filePath)
		if err != nil {
			return nil, err
		}
		componentsSet.Append(components...)
	}

	if componentsSet.Cardinality() == 0 {
		return nil, fmt.Errorf("no components found in the stack files")
	}

	// 4. get all the resources from the terraform config files
	resources, err := t.ListAllResourcesFromWorkspaceState(terraformConfigFilesAbsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list resources from workspace state: %v", err)
	}

	if isFullyModular := t.IsFullyModular(resources); !isFullyModular {
		if componentsSet.Cardinality() != 1 {
			return nil, fmt.Errorf("the Terraform state is not fully modular, found %d components, expected 1", componentsSet.Cardinality())
		}
		stackComponentAddress := fmt.Sprintf("%s.%s", "component", componentsSet.ToSlice()[0])
		for _, resource := range resources {

			workspaceToStackAddressMap[resource] = stackComponentAddress
		}

		return workspaceToStackAddressMap, nil
	}

	// 5. If the state is fully modular, get all the top-level modules
	topLevelModules, err := t.getTopLevelModules(resources)
	if err != nil {
		return nil, fmt.Errorf("failed to get top-level modules, err: %v", err)
	}

	// 6. components name must match the top-level module names
	if topLevelModules.SymmetricDifference(componentsSet).Cardinality() != 0 {
		return nil, fmt.Errorf("the top-level modules %v do not match the components %v", topLevelModules.ToSlice(), componentsSet.ToSlice())
	}

	for _, topLevelModule := range topLevelModules.ToSlice() {
		workspaceToStackAddressMap[topLevelModule] = topLevelModule
	}

	return workspaceToStackAddressMap, nil
}

// validateStacksFiles checks if the provided path contains valid stack configuration files.
func (t *tfWorkspaceStateUtility) validateStacksFiles(stackSourceBundleAbsPath string) (bool, error) {
	// Check if the path exists and is a directory
	info, err := os.Stat(stackSourceBundleAbsPath)
	if os.IsNotExist(err) {
		return false, fmt.Errorf("path %s does not exist", stackSourceBundleAbsPath)
	}
	if err != nil {
		return false, fmt.Errorf("error accessing path %s: %v", stackSourceBundleAbsPath, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("path %s is not a directory", stackSourceBundleAbsPath)
	}

	cmd := exec.CommandContext(t.ctx, "terraform", "stacks", "validate")
	cmd.Dir = stackSourceBundleAbsPath

	// Remove TF_LOG and TF_CLI_CONFIG_FILE from the environment for this command
	env := os.Environ()
	var filteredEnv []string
	for _, e := range env {
		if !strings.HasPrefix(e, "TF_LOG=") && !strings.HasPrefix(e, "TF_CLI_CONFIG_FILE=") {
			filteredEnv = append(filteredEnv, e)
		}
	}
	cmd.Env = filteredEnv

	_, err = cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return false, fmt.Errorf("failed to validate stacks: %s", string(exitErr.Stderr))
		}
		return false, fmt.Errorf("failed to validate stacks: %w", err)
	}

	return true, nil
}

// getStackFiles retrieves all stack files from the specified directory.
func (t *tfWorkspaceStateUtility) getStackFiles(stackSourceBundleAbsPath string) ([]string, error) {
	filePathGlobPattern := fmt.Sprintf("%s%s*%s", stackSourceBundleAbsPath, string(os.PathSeparator), stackComponentHCLFileExt)
	stackFiles, err := filepath.Glob(filePathGlobPattern)
	if err != nil {
		return nil, fmt.Errorf("error while fetching stack files from path %s, err: %w", stackSourceBundleAbsPath, err)
	}

	if len(stackFiles) == 0 {
		return nil, fmt.Errorf("no stack files found in the directory %s", stackSourceBundleAbsPath)
	}

	return stackFiles, nil
}

// getAllComponents retrieves all components from the specified HCL file.
func (t *tfWorkspaceStateUtility) getAllComponents(filePath string) ([]string, error) {
	// parse the hcl file at the given filePath
	file, diags := t.hclParser.ParseHCLFile(filePath)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL file %s, err: %v", filePath, diags.Error())
	}

	// check if the file is nil or has no body
	if file == nil || file.Body == nil {
		return nil, nil
	}

	// define the schema to extract blocks of type "component" with a label "name"
	schema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{
				Type:       "component",
				LabelNames: []string{"name"},
			},
		},
	}

	// use PartialContent to get the content of the file that matches the schema
	// this will return the blocks of type "component" with their labels
	// it is important that we use PartialContent here,
	// as the parsed file may contain other blocks that we are not interested in
	content, _, diags := file.Body.PartialContent(schema)
	if diags.HasErrors() {
		return nil, diags
	}

	// check if the content is nil or has no content blocks
	// if so, return nil
	if content == nil || len(content.Blocks) == 0 {
		return nil, nil
	}

	var components []string

	// let us iterate through the blocks and extract the labels
	// we assume that each block of a type "component" has one label (the name)
	// if there are multiple labels, we will only take the first one
	// we also assume that we have exactly one distinct label per component block
	for _, block := range content.Blocks {
		// The block type is "component" and it has one label (the name)
		components = append(components, block.Labels[0])
	}

	return components, nil
}

// getTopLevelModules retrieves all top-level modules from the provided resources.
func (t *tfWorkspaceStateUtility) getTopLevelModules(resources []string) (topLevelModules mapset.Set[string], err error) {
	topLevelChildModules := mapset.NewSet[string]()
	moduleRegex := regexp.MustCompile(firstLevelModuleExpression)
	for _, resource := range resources {
		if matches := moduleRegex.FindStringSubmatch(resource); matches != nil {
			// matches[1] contain the module name
			topLevelChildModules.Add(matches[1])
		}
	}

	if topLevelChildModules.Cardinality() == 0 {
		return nil, fmt.Errorf("no top-level modules found in the resources")
	}

	return topLevelChildModules, nil
}
