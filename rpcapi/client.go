// Copyright IBM Corp. 2025
// SPDX-License-Identifier: BUSL-1.1

package rpcapi

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	"github.com/hashicorp/terraform-migrate-utility/rpcapi/terraform1/dependencies"
	"github.com/hashicorp/terraform-migrate-utility/rpcapi/terraform1/packages"
	"github.com/hashicorp/terraform-migrate-utility/rpcapi/terraform1/setup"
	"github.com/hashicorp/terraform-migrate-utility/rpcapi/terraform1/stacks"
)

const (
	TerraformRPCAPICookie            string = "fba0991c9bcd453982f0d88e2da95940"
	TerraformMagicCookieKey          string = "TERRAFORM_RPCAPI_COOKIE"
	UnsupportedTerraformVersionError        = `
The Terraform Stacks is only compatible with specific Terraform versions.

For supported Terraform versions, refer to: https://hashi.co/tfstacks-requirements
`
)

var (
	_ Client            = (*grpcClient)(nil)
	_ plugin.Plugin     = (*TerraformPlugin)(nil)
	_ plugin.GRPCPlugin = (*TerraformPlugin)(nil)
)

type grpcClient struct {
	conn         *grpc.ClientConn
	dependencies dependencies.DependenciesClient
	packages     packages.PackagesClient
	pluginClient *plugin.Client
	stacks       stacks.StacksClient
}
type TerraformPlugin struct {
	plugin.NetRPCUnsupportedPlugin
}

type Client interface {
	Dependencies() dependencies.DependenciesClient
	Packages() packages.PackagesClient
	Stacks() stacks.StacksClient
	Stop()
}

// NewTerraformRpcClient creates a new Terraform gRPC client with the provided context, initializing the associated plugin client.
// Returns a Client interface or an error if the setup process fails.
func NewTerraformRpcClient(ctx context.Context) (Client, error) {

	cmd := exec.CommandContext(ctx, "terraform", "rpcapi")
	cmd.Dir = "."

	config := &plugin.ClientConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   TerraformMagicCookieKey,
			MagicCookieValue: TerraformRPCAPICookie,
		},
		Cmd:              cmd,
		AutoMTLS:         true,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Managed:          false,
		Plugins: map[string]plugin.Plugin{
			"terraform": &TerraformPlugin{},
		},
	}

	client := plugin.NewClient(config)
	if _, err := client.Start(); err != nil {
		return nil, err
	}

	protocol, err := client.Client()
	if err != nil {
		return nil, err
	}

	raw, err := protocol.Dispense("terraform")
	if err != nil {
		return nil, err
	}

	grpcClient := raw.(*grpcClient)
	grpcClient.pluginClient = client
	return grpcClient, err
}

// Dependencies return the DependenciesClient instance, initializing it if not already created.
func (g *grpcClient) Dependencies() dependencies.DependenciesClient {
	if g.dependencies == nil {
		g.dependencies = dependencies.NewDependenciesClient(g.conn)
	}
	return g.dependencies
}

// Packages initialize and return a PackagesClient instance if not already created.
func (g *grpcClient) Packages() packages.PackagesClient {
	if g.packages == nil {
		g.packages = packages.NewPackagesClient(g.conn)
	}
	return g.packages
}

// Stacks initializes and returns a StacksClient instance if not already created.
func (g *grpcClient) Stacks() stacks.StacksClient {
	if g.stacks == nil {
		g.stacks = stacks.NewStacksClient(g.conn)
	}
	return g.stacks
}

// Stop terminates the gRPC client connection and cleans up the plugin client.
// This method is used to gracefully shut down the client connection and release resources.
// It should be called when the client is no longer needed to prevent resource leaks.
func (g *grpcClient) Stop() {
	g.pluginClient.Kill()
}

// GRPCClient establishes a gRPC client connection for the Terraform plugin and performs a setup handshake process.
// Returns a client interface if successful, or an error if the handshake fails.
func (t *TerraformPlugin) GRPCClient(ctx context.Context, _ *plugin.GRPCBroker, conn *grpc.ClientConn) (interface{}, error) {
	client := setup.NewSetupClient(conn)
	_, err := client.Handshake(ctx, &setup.Handshake_Request{})
	if err != nil {
		return nil, fmt.Errorf("rpcapi setup handshake failed: %v", err)
	}

	return &grpcClient{
		conn: conn,
	}, nil
}

// GRPCServer returns an error as this implementation only supports client gRPC connections and not server creation.
func (t *TerraformPlugin) GRPCServer(_ *plugin.GRPCBroker, _ *grpc.Server) error {
	// Nowhere in this codebase should we try and launch a server anyway.
	return fmt.Errorf("stacks only supports client gRPC connections")
}
