// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tfstateutil

// This code has been kept as a reference for the previous implementation of the tfWorkspaceStateUtility.
/*
import (
	"context"
	"fmt"
	"github.com/deckarep/golang-set/v2"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/tidwall/gjson"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

const (
	managedResourcesPath       = `resources.#(mode=="managed")#`
	managedDataPath            = `resources.#(mode=="data")#`
	moduleKey                  = `module`
	stackComponentHCLFileExt   = `.tfcomponent.hcl`
	firstLevelModuleExpression = `^module\.([^.]+)`
)

type tfWorkspaceStateUtility struct {
	ctx       context.Context
	hclParser *hclparse.Parser
}

// TfWorkspaceStateUtility defines the interface for operations related to Terraform workspace state.
type TfWorkspaceStateUtility interface {
	GetManagedResources(workspaceStateData []byte) ([]byte, error)
	IsFullyModular(workspaceStateData []byte) bool
	WorkspaceToStackAddressMap(managedResourcesJson []byte, dataResourcesJson []byte, stackSourceBundleAbsPath string) (map[string]string, map[string]string, error)
	GetDataResources(workspaceStateData []byte) ([]byte, error)
}

// NewTfWorkspaceStateUtility creates a new instance of TfWorkspaceStateUtility with the provided context.
// This utility is used to handle operations related to Terraform workspace state.
func NewTfWorkspaceStateUtility(ctx context.Context, parser *hclparse.Parser) TfWorkspaceStateUtility {
	return &tfWorkspaceStateUtility{
		ctx:       ctx,
		hclParser: parser,
	}
}

// GetDataResources retrieves all data resources from the provided workspace state data.
// add an error handling when the workspace state data is not valid JSON or
// does not have any data resources.
func (tf *tfWorkspaceStateUtility) GetDataResources(workspaceStateData []byte) ([]byte, error) {
	if !gjson.Valid(string(workspaceStateData)) {
		return nil, fmt.Errorf("invalid workspace state data, expected valid JSON")
	}
	// use gjson to get all data resources from the workspace state data
	// the path is defined as resources.#(mode=="data")
	dataResources := gjson.GetBytes(workspaceStateData, managedDataPath)
	if !dataResources.Exists() {
		return nil, fmt.Errorf("no data resources found in the workspace state data")
	}

	// return the array of data resources
	return []byte(dataResources.Raw), nil
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
func (tf *tfWorkspaceStateUtility) WorkspaceToStackAddressMap(managedResourcesJson []byte, dataResourcesJson []byte, stackSourceBundleAbsPath string) (map[string]string, map[string]string, error) {
	// 1. Validate the stack source bundle path
	if _, err := validateStacksFiles(stackSourceBundleAbsPath); err != nil {
		return nil, nil, err
	}

	// 2. Get all stack files from the stack source bundle path
	stackFiles, err := getStackFiles(stackSourceBundleAbsPath)
	if err != nil {
		fmt.Printf("Error getting stack files: %v\n", err)
		return nil, nil, err
	}

	// 3. Create a set to hold all components from the stack files
	componentsSet := mapset.NewSet[string]()

	// 4. Iterate through each stack file and extract components
	for _, filePath := range stackFiles {
		components, err := tf.getAllComponents(filePath)
		if err != nil {
			return nil, nil, err
		}
		componentsSet.Append(components...)
	}

	if componentsSet.Cardinality() == 0 {
		return nil, nil, fmt.Errorf("no components found in the stack files")
	}

	// 6.Check if all the managed resources have a module address
	fullyModular := tf.IsFullyModular(managedResourcesJson)

	// 5. Get all modules from the managed resources JSON
	modulesSet := getModulesFromWorkspaceState(managedResourcesJson)

	// 7. Validate the components and modules sets
	// This validation is crucial in scenarios where the Terraform configuration files governing HCP Terraform workspaces have been fully modularized,
	// but the corresponding workspace state files have not yet been updated to reflect this modularization.
	// In such cases, all managed resources should be mapped to a single stack component address.
	// This typically occurs when a previously non-modularized HCP Terraform workspace configuration has undergone modularization using the `tfmigrate` tool.
	// As a result, the configuration files become fully modularized with a single module, while the state files remain in their original, non-modularized format.
	// This approach ensures that stack configuration files are generated only when the Terraform configuration files are fully modularized.
	if !fullyModular && componentsSet.Cardinality() > 1 {
		return nil, nil, fmt.Errorf("the workspace is not fully modular, but multiple components found: %v", componentsSet.ToSlice())
	}

	// 8. Check if the components and modules sets have any differences
	// If the workspace is fully modular, the component names must match the module names.
	// If there are any differences, return an error.
	hasDifference := (modulesSet.Difference(componentsSet).Cardinality() > 0) || (componentsSet.Difference(modulesSet).Cardinality() > 0)
	if fullyModular && hasDifference {
		return nil, nil, fmt.Errorf("the workspace is fully modular componet names must match module names, componets set: %v, modules set: %v", componentsSet.ToSlice(), modulesSet.ToSlice())
	}

	rootResourceAddressMap := make(map[string]string)
	moduleAddressMap := make(map[string]string)
	if !fullyModular {
		rootModuleResourceAddresses := getRootModuleResourceAddresses(managedResourcesJson)
		rootModuleDataResources := getRootModuleDataAddresses(dataResourcesJson)

		componentName := componentsSet.ToSlice()[0]
		if rootModuleResourceAddresses.Cardinality() > 0 {
			for _, rootModuleResource := range rootModuleResourceAddresses.ToSlice() {
				rootResourceAddressMap[rootModuleResource] = "component." + componentName
			}
		}

		if rootModuleDataResources.Cardinality() > 0 {
			for _, rootModuleData := range rootModuleDataResources.ToSlice() {
				rootResourceAddressMap[rootModuleData] = "component." + componentName
			}
		}

		for _, module := range modulesSet.ToSlice() {
			moduleAddressMap[module] = componentName
		}

	} else {
		// If the workspace is fully modular, we map each component to its corresponding module address
		for _, component := range componentsSet.ToSlice() {
			moduleAddressMap[component] = component
		}
	}

	return rootResourceAddressMap, moduleAddressMap, nil
}

func getStackFiles(stackSourceBundleAbsPath string) ([]string, error) {
	filePathGlobPattern := fmt.Sprintf("%s%s*%s", stackSourceBundleAbsPath, string(os.PathSeparator), stackComponentHCLFileExt)
	files, err := filepath.Glob(filePathGlobPattern)
	if err != nil {
		return nil, fmt.Errorf("error while fetching stack files from path %s, err: %w", stackSourceBundleAbsPath, err)
	}
	return files, nil
}

func (tf *tfWorkspaceStateUtility) getAllComponents(filePath string) ([]string, error) {
	// parse the hcl file at the given filePath
	file, diags := tf.hclParser.ParseHCLFile(filePath)
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

func validateStacksFiles(stackSourceBundleAbsPath string) (bool, error) {
	// Check if the path exists
	if _, err := os.Stat(stackSourceBundleAbsPath); os.IsNotExist(err) {
		return false, fmt.Errorf("path %s does not exist", stackSourceBundleAbsPath)
	}

	// Check if the path is a directory
	if info, err := os.Stat(stackSourceBundleAbsPath); err != nil || !info.IsDir() {
		return false, fmt.Errorf("path %s is not a directory", stackSourceBundleAbsPath)
	}

	cmd := exec.CommandContext(context.Background(), "terraform", "stacks", "validate")
	cmd.Dir = stackSourceBundleAbsPath
	_, err := cmd.Output()
	if err != nil {
		return false, err
	}

	return true, nil
}

func getModulesFromWorkspaceState(managedResourcesJson []byte) mapset.Set[string] {
	modulesSet := mapset.NewSet[string]()
	managedResources := gjson.ParseBytes(managedResourcesJson).Array()
	for _, resource := range managedResources {
		// check if the resource has a module address
		if !resource.Get(moduleKey).Exists() {
			continue
		}

		moduleAddress := resource.Get(moduleKey).String()
		mod := extractFirstLevelModule(moduleAddress)
		if mod != "" {
			modulesSet.Add(mod)
		}
	}
	return modulesSet
}

func extractFirstLevelModule(input string) string {
	re := regexp.MustCompile(firstLevelModuleExpression)
	matches := re.FindStringSubmatch(input)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func getRootModuleResourceAddresses(managedResourcesJson []byte) mapset.Set[string] {
	rootModuleResourceAddresses := mapset.NewSet[string]()
	for _, resource := range gjson.ParseBytes(managedResourcesJson).Array() {
		if !resource.Get(moduleKey).Exists() {
			instancesCount := resource.Get("instances.#").Int()
			resourceAddress := resource.Get("type").String() + "." + resource.Get("name").String()
			if instancesCount == 1 {
				rootModuleResourceAddresses.Add(resourceAddress)
				continue
			}
			for i := 0; i < int(instancesCount); i++ {
				rootModuleResourceAddresses.Add(fmt.Sprintf("%s[%d]", resourceAddress, i))
			}
		}
	}

	return rootModuleResourceAddresses
}

func getRootModuleDataAddresses(dataResourcesJson []byte) mapset.Set[string] {
	rootModuleDataAddresses := mapset.NewSet[string]()
	for _, resource := range gjson.ParseBytes(dataResourcesJson).Array() {
		if !resource.Get(moduleKey).Exists() {
			instancesCount := resource.Get("instances.#").Int()
			resourceAddress := resource.Get("mode").String() + "." + resource.Get("type").String() + "." + resource.Get("name").String()
			if instancesCount == 1 {
				rootModuleDataAddresses.Add(resourceAddress)
				continue
			}
			for i := 0; i < int(instancesCount); i++ {
				rootModuleDataAddresses.Add(fmt.Sprintf("%s[%d]", resourceAddress, i))
			}
		}
	}

	return rootModuleDataAddresses
}
*/
