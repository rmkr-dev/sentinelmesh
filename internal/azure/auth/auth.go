// Package auth selects an Azure credential. Production refuses the Azure CLI credential.
package auth

import (
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// New returns a credential for the configured mode.
// Modes: workload_identity, managed_identity, azure_cli.
func New(mode, environment string) (azcore.TokenCredential, error) {
	if mode == "" {
		mode = "workload_identity"
	}
	if mode == "azure_cli" && environment == "production" {
		return nil, fmt.Errorf("azure_cli credentials are refused when environment=production")
	}
	switch mode {
	case "workload_identity":
		return azidentity.NewWorkloadIdentityCredential(nil)
	case "managed_identity":
		return azidentity.NewManagedIdentityCredential(nil)
	case "azure_cli":
		return azidentity.NewAzureCLICredential(nil)
	default:
		return nil, fmt.Errorf("unknown azure auth mode %q", mode)
	}
}

// ModeFromEnv reads AZURE_AUTH and falls back to the argument.
func ModeFromEnv(fallback string) string {
	if v := os.Getenv("AZURE_AUTH"); v != "" {
		return v
	}
	return fallback
}
