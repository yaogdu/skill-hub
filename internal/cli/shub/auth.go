package shub

import "fmt"

func requireSHUBTokenForRegistryRead() error {
	if apiClient == nil {
		return fmt.Errorf("API client not initialized")
	}
	if apiClient.HasToken() {
		return nil
	}
	settings, err := apiClient.GetRegistryAuthSettings()
	if err == nil && settings != nil && !settings.APIKeyValidationEnabled {
		return nil
	}
	return fmt.Errorf("SHUB API key is required; set SHUB_API_TOKEN or ARCTL_API_TOKEN to continue")
}

func requireSHUBTokenForPublish() error {
	if apiClient == nil {
		return fmt.Errorf("API client not initialized")
	}
	if apiClient.HasToken() {
		return nil
	}
	return fmt.Errorf("SHUB API key is required for publish operations; set SHUB_API_TOKEN or ARCTL_API_TOKEN to continue")
}
