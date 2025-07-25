# Terraform Migrate Utility

A Go library and utility for migrating Terraform state files to Terraform Stacks using HashiCorp's Terraform RPC API.

## Overview

This utility provides a Go-based interface to migrate traditional Terraform configurations and state files to the new Terraform Stacks format. It leverages Terraform's gRPC-based RPC API to perform the migration operations safely and efficiently.

## Features

- **State Migration**: Migrate existing Terraform state files to Terraform Stacks format
- **Configuration Handling**: Process Terraform modules, dependencies, and provider configurations
- **gRPC Integration**: Uses Terraform's official RPC API for reliable operations
- **Resource Mapping**: Supports custom resource and module address mapping during migration
- **Provider Cache Management**: Handles provider plugin caches and dependency locks

## Requirements

- Go 1.24.2 or later
- Terraform CLI with RPC API support
- Compatible Terraform version (refer to [Terraform Stacks requirements](https://hashi.co/tfstacks-requirements))

## Installation

```bash
go get terraform-migrate-utility
```

## Usage

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "terraform-migrate-utility/rpcapi"
    "terraform-migrate-utility/rpcapi/terraform1/stacks"
    stateOps "terraform-migrate-utility/stateops"

    "google.golang.org/protobuf/encoding/protojson"
)

var (
    jsonOpts = protojson.MarshalOptions{
        Multiline: true,
    }
)

func main() {
    ctx := context.Background()

    // Connect to Terraform RPC API
    client, err := rpcapi.Connect(ctx)
    if err != nil {
        fmt.Println("Error connecting to RPC API:", err)
        return
    }
    defer client.Stop()

    // Create state operations handler
    r := stateOps.NewTFStateOperations(ctx, client)

    // Define paths (adjust these to your project structure)
    workspaceDir := "/path/to/your/terraform/workspace"
    stackConfigDir := "/path/to/your/stack/config"

    // Open Terraform state from workspace directory
    tfStateHandle, closeTFState, err := r.OpenTerraformState(workspaceDir)
    if err != nil {
        fmt.Println("Error opening Terraform state:", err)
        return
    }
    defer closeTFState()

    // Open source bundle (modules directory)
    sourceBundleHandle, closeSourceBundle, err := r.OpenSourceBundle(filepath.Join(workspaceDir, ".terraform/modules/"))
    if err != nil {
        fmt.Println("Error opening source bundle:", err)
        return
    }
    defer closeSourceBundle()

    // Open stack configuration (relative path from current working directory)
    cwd, _ := os.Getwd()
    relStackPath, _ := filepath.Rel(cwd, stackConfigDir)
    stackConfigHandle, closeConfig, err := r.OpenStacksConfiguration(sourceBundleHandle, relStackPath)
    if err != nil {
        fmt.Println("Error opening stacks configuration:", err)
        return
    }
    defer closeConfig()

    // Open dependency lock file (relative path from current working directory)
    lockFilePath := filepath.Join(workspaceDir, ".terraform.lock.hcl")
    relLockPath, _ := filepath.Rel(cwd, lockFilePath)
    dependencyLocksHandle, closeLock, err := r.OpenDependencyLockFile(sourceBundleHandle, relLockPath)
    if err != nil {
        fmt.Println("Error opening dependency lock file:", err)
        return
    }
    defer closeLock()

    // Open provider cache
    providerCacheHandle, closeProviderCache, err := r.OpenProviderCache(filepath.Join(workspaceDir, ".terraform/providers"))
    if err != nil {
        fmt.Println("Error opening provider cache:", err)
        return
    }
    defer closeProviderCache()

    // Perform migration with custom mappings
    events, err := r.MigrateTFState(
        tfStateHandle,
        stackConfigHandle,
        dependencyLocksHandle,
        providerCacheHandle,
        map[string]string{}, // Resource mappings (empty in this example)
        map[string]string{   // Module mappings
            "random_number":   "triage-min",
            "random_number_2": "triage-max",
        },
    )
    if err != nil {
        fmt.Println("Error migrating Terraform state:", err)
        return
    }

    // Process migration events
    for {
        item, err := events.Recv()
        if err == io.EOF {
            break // Migration completed successfully
        } else if err != nil {
            fmt.Println("Error receiving migration events:", err)
            return
        }

        // Handle different event types
        switch result := item.Result.(type) {
        case *stacks.MigrateTerraformState_Event_AppliedChange:
            for _, change := range result.AppliedChange.Descriptions {
				stackState := &tfstacksagent1.StackState{
					FormatVersion: 1,
					Raw:           make(map[string]*anypb.Any),
					Descriptions: map[string]*stacks.AppliedChange_ChangeDescription{
						"change": change,
					},
				}
				// if err := tfstacksagent1.WriteStateSnapshot(os.Stdout, stackState); err != nil {
				// 	fmt.Println("Error writing state snapshot:", err)
				// }
				// fmt.Println(jsonOpts.Format(stackState))
			}
        case *stacks.MigrateTerraformState_Event_Diagnostic:
            fmt.Println("Diagnostic:", result.Diagnostic.Detail)
        default:
            fmt.Printf("Received event: %T\n", result)
        }
    }

    fmt.Println("Migration completed successfully!")
}
```


## Contributing

This project follows HashiCorp's contribution guidelines. Please ensure:

1. Code follows Go best practices
2. All changes include appropriate tests
3. Documentation is updated for API changes
4. Protobuf changes are properly generated
