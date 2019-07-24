package azure

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/preview/frontdoor/mgmt/2019-04-01/frontdoor"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/validate"
)

//Frontdoor name must begin with a letter or number, end with a letter or number and may contain only letters, numbers or hyphens.
func ValidateFrontDoorName(i interface{}, k string) (_ []string, errors []error) {
	if m, regexErrs := validate.RegExHelper(i, k, `(^[\da-zA-Z])([-\da-zA-Z]{3,61})([\da-zA-Z]$)`); !m {
		errors = append(regexErrs, fmt.Errorf(`%q must be between 5 and 63 characters in length and begin with a letter or number, end with a letter or number and may contain only letters, numbers or hyphens.`, k))
	}

	return nil, errors
}

func ValidateBackendPoolRoutingRuleName(i interface{}, k string) (_ []string, errors []error) {
	if m, regexErrs := validate.RegExHelper(i, k, `(^[\da-zA-Z])([-\da-zA-Z]{1,88})([\da-zA-Z]$)`); !m {
		errors = append(regexErrs, fmt.Errorf(`%q must be between 1 and 90 characters in length and begin with a letter or number, end with a letter or number and may contain only letters, numbers or hyphens.`, k))
	}

	return nil, errors
}

func ValidateFrontdoor(d *schema.ResourceData) error {
	routingRules := d.Get("routing_rule").([]interface{})
	configFrontendEndpoints := d.Get("frontend_endpoint").([]interface{})
	backendPools := d.Get("backend_pool").([]interface{})
	loadBalancingSettings:= d.Get("backend_pool_load_balancing").([]interface{})
	healthProbeSettings:= d.Get("backend_pool_health_probe").([]interface{})

	if len(configFrontendEndpoints) == 0 {
		return fmt.Errorf(`"frontend_endpoint": must have at least one "frontend_endpoint" defined, found 0`)
	}
	
	if routingRules != nil {
		// Loop over all of the Routing Rules and validate that only one type of configuration is defined per Routing Rule
		for _, rr := range routingRules {
			routingRule := rr.(map[string]interface{})
			routingRuleName := routingRule["name"]
			found := false

			redirectConfig := routingRule["redirect_configuration"].([]interface{})
			forwardConfig := routingRule["forwarding_configuration"].([]interface{})

			// Check 1. validate that only one configuration type is defined per routing rule
			if len(redirectConfig) == 1 && len(forwardConfig) == 1 {
				return fmt.Errorf(`"routing_rule":%q is invalid. "redirect_configuration" conflicts with "forwarding_configuration". You can only have one configuration type per routing rule`, routingRuleName)
			}

			// Check 2. validate that each routing rule frontend_endpoints are actually defined in the resource schema
			if routingRuleFrontends := routingRule["frontend_endpoints"].([]interface{}); len(routingRuleFrontends) > 0 {

				for _, routingRuleFrontend := range routingRuleFrontends {
					// Get the name of the frontend defined in the routing rule
					routingRulefrontendName := routingRuleFrontend.(string)
					found = false

					// Loop over all of the defined frontend endpoints in the config 
					// seeing if we find the routing rule frontend in the list
					for _, configFrontendEndpoint := range configFrontendEndpoints {
						configFrontend := configFrontendEndpoint.(map[string]interface{})
						configFrontendName := configFrontend["name"]
						if( routingRulefrontendName == configFrontendName){
							found = true
							break
						}
					}

					if !found {
						return fmt.Errorf(`"routing_rule":%q "frontend_endpoints":%q was not found in the configuration file. verify you have the "frontend_endpoint":%q defined in the configuration file`, routingRuleName, routingRulefrontendName, routingRulefrontendName)
					}
				}
			} else {
				return fmt.Errorf(`"routing_rule": %q must have at least one "frontend_endpoints" defined`, routingRuleName)
			}
		}
	}

	// Verify backend pool load balancing settings and health probe settings are defined in the resource schema
	if backendPools != nil {
		
		for _, bp := range backendPools {
			backendPool := bp.(map[string]interface{})
			backendPoolName := backendPool["name"]
			backendPoolLoadBalancingName := backendPool["load_balancing_name"]
			backendPoolHealthProbeName := backendPool["health_probe_name"]
			found := false

			// Verify backend pool load balancing settings name exists
			for _, lbs := range loadBalancingSettings{
				loadBalancing := lbs.(map[string]interface{})
				loadBalancingName := loadBalancing["name"]

				if loadBalancingName == backendPoolLoadBalancingName {
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf(`"backend_pool":%q "load_balancing_name":%q was not found in the configuration file. verify you have the "backend_pool_load_balancing":%q defined in the configuration file`, backendPoolName, backendPoolLoadBalancingName, backendPoolLoadBalancingName)
			}

			found = false

			// Verify health probe settings name exists
			for _, hps := range healthProbeSettings{
				healthProbe := hps.(map[string]interface{})
				healthProbeName := healthProbe["name"]

				if healthProbeName == backendPoolHealthProbeName {
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf(`"backend_pool":%q "health_probe_name":%q was not found in the configuration file. verify you have the "backend_pool_health_probe":%q defined in the configuration file`, backendPoolName, backendPoolHealthProbeName, backendPoolHealthProbeName)
			}
			
		}
	} else {
		return fmt.Errorf(`"backend_pool": must have at least one "backend" defined`)
	}

	// Verify frontend endpoints custom https configuration is valid if defined
	for _, configFrontendEndpoint := range configFrontendEndpoints {
		if configFrontend := configFrontendEndpoint.(map[string]interface{}); len(configFrontend) > 0 {
			FrontendName := configFrontend["name"]
			if chc := configFrontend["custom_https_configuration"].([]interface{}); len(chc) > 0  { 
				customHttpsConfiguration := chc[0].(map[string]interface{})
				certificateSource := customHttpsConfiguration["certificate_source"]
				if certificateSource == string(frontdoor.CertificateSourceAzureKeyVault) {
					if !azureKeyVaultCertificateHasValues(customHttpsConfiguration, true) {
						return fmt.Errorf(`"frontend_endpoint":%q "custom_https_configuration" is invalid, all of the following keys must have values in the "custom_https_configuration" block: "azure_key_vault_certificate_secret_name", "azure_key_vault_certificate_secret_version", and "azure_key_vault_certificate_vault_id"`, FrontendName)
					}
				} else {
					if azureKeyVaultCertificateHasValues(customHttpsConfiguration, false) {
						return fmt.Errorf(`"frontend_endpoint":%q "custom_https_configuration" is invalid, all of the following keys must be removed from the "custom_https_configuration" block: "azure_key_vault_certificate_secret_name", "azure_key_vault_certificate_secret_version", and "azure_key_vault_certificate_vault_id"`, FrontendName)
					}
				}
			}
		}
	}

	return nil
}

func azureKeyVaultCertificateHasValues(customHttpsConfiguration map[string]interface{}, MatchAllKeys bool) bool {
	certificateSecretName := customHttpsConfiguration["azure_key_vault_certificate_secret_name"]
	certificateSecretVersion := customHttpsConfiguration["azure_key_vault_certificate_secret_version"]
	certificateVaultId := customHttpsConfiguration["azure_key_vault_certificate_vault_id"]

	if MatchAllKeys {
		if strings.TrimSpace(certificateSecretName.(string)) != "" && strings.TrimSpace(certificateSecretVersion.(string)) != ""  && strings.TrimSpace(certificateVaultId.(string))  != "" {
			return true
		}
	} else {
		if strings.TrimSpace(certificateSecretName.(string)) != "" || strings.TrimSpace(certificateSecretVersion.(string)) != ""  || strings.TrimSpace(certificateVaultId.(string)) != "" {
			return true
		}
	}
	
	return false
}

func GetFrontDoorSubResourceId (subscriptionId string, resourceGroup string, serviceName string, resourceType string, resourceName string) string {
	if strings.TrimSpace(subscriptionId) == "" || strings.TrimSpace(resourceGroup) == "" || strings.TrimSpace(serviceName) == "" || strings.TrimSpace(resourceType) == "" || strings.TrimSpace(resourceName) == "" {
		return ""
	}

	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/frontDoors/%s/%s/%s", subscriptionId, resourceGroup, serviceName, resourceType, resourceName)
}
