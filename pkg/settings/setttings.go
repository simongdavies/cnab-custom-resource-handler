package settings

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"get.porter.sh/porter/pkg/porter"

	"github.com/cnabio/cnab-go/bundle"
	"github.com/cnabio/cnab-to-oci/remotes"
	"github.com/docker/cli/cli/config"
	"github.com/docker/distribution/reference"
	log "github.com/sirupsen/logrus"
)

var IsRPaaS bool
var LogRequestBody bool

var RequiredSettings = map[string]string{
	"StorageAccountName":   "CNAB_AZURE_STATE_STORAGE_ACCOUNT_NAME",
	"StorageResourceGroup": "CNAB_AZURE_STATE_STORAGE_RESOURCE_GROUP",
	"SusbcriptionId":       "CNAB_AZURE_SUBSCRIPTION_ID",
	"BundleTag":            "CNAB_BUNDLE_TAG",
	"AsyncOpTable":         "CUSTOM_RP_ASYNC_OP_TABLE",
	"StateTable":           "CUSTOM_RP_STATE_TABLE",
}

var OptionalSettings = map[string]interface{}{
	"AllowInsecureRegistry": "CNAB_BUNDLE_INSECURE_REGISTRY:bool",
	"ForcePull":             "CNAB_BUNDLE_FORCE_PULL:bool",
	"LogRequestBody":        "LOG_REQUEST_BODY:bool",
	"ResourceProvider":      "RESOURCE_PROVIDER:string",
	"ResourceType":          "RESOURCE_TYPE:string",
}

type BundleInformation struct {
	ResourceType      string
	ResourceProvider  string
	BundlePullOptions *porter.BundlePullOptions
	TrimmedBundleTag  string
	RPBundle          *bundle.Bundle
}

var RPToProvider = make(map[string]*BundleInformation)

// var BundleInfo BundleInformation

func Load() error {
	for k, v := range RequiredSettings {
		val := os.Getenv(v)
		if len(val) == 0 {
			return fmt.Errorf("Environment Variable %s is not set", v)
		}
		RequiredSettings[k] = strings.TrimSpace(val)
	}
	for k, v := range OptionalSettings {
		parts := strings.Split(v.(string), ":")
		val := os.Getenv(parts[0])
		switch parts[1] {
		case "bool":
			if boolVal, err := strconv.ParseBool(val); err == nil {
				OptionalSettings[k] = boolVal
			} else {
				OptionalSettings[k] = false
			}
		case "string":
			OptionalSettings[k] = strings.TrimSpace(val)
		default:
			OptionalSettings[k] = false
		}
	}

	resourceProviderName := OptionalSettings["ResourceProvider"].(string)
	resourceTypeName := OptionalSettings["ResourceType"].(string)
	IsRPaaS = len(resourceProviderName) > 0 && !strings.EqualFold("Microsoft.CustomProviders", resourceProviderName)

	//TODO properly handle RPaaS
	if IsRPaaS {
		// TODO something
	} else {
		bundleInformation := BundleInformation{
			ResourceProvider: resourceProviderName,
			ResourceType:     resourceTypeName,
		}
		rpType := GetRPName(resourceProviderName, resourceTypeName)
		RPToProvider[rpType] = &bundleInformation
		ref, err := validateBundleTag(RequiredSettings["BundleTag"])
		if err != nil {
			log.Errorf("Error validating bundle tag %v", err)
			return err
		}
		// TODO need to handle digests and versioning correctly
		if _, ok := ref.(reference.Tagged); !ok {
			if _, ok := ref.(reference.Digested); !ok {
				RequiredSettings["BundleTag"] += ":latest"
			}
		}

		bundleInformation.TrimmedBundleTag = reference.TrimNamed(ref).String()
		bundleInformation.BundlePullOptions = &porter.BundlePullOptions{
			Tag:              RequiredSettings["BundleTag"],
			Force:            OptionalSettings["ForcePull"].(bool),
			InsecureRegistry: OptionalSettings["AllowInsecureRegistry"].(bool),
		}

		if err := pullBundle(&bundleInformation); err != nil {
			log.Errorf("Error pulling bundle %v", err)
			return err
		}

	}

	LogRequestBody = OptionalSettings["LogRequestBody"].(bool)

	return nil
}

// GetRPName returns the RP Name
func GetRPName(resourceProviderName string, resourceTypeName string) string {
	return fmt.Sprintf("%s/%s", resourceProviderName, resourceTypeName)
}

func pullBundle(bundleInfo *BundleInformation) error {
	ref, err := reference.ParseNormalizedNamed(bundleInfo.BundlePullOptions.Tag)
	if err != nil {
		return fmt.Errorf("Invalid bundle tag format %s, expected REGISTRY/name:tag %w", bundleInfo.BundlePullOptions.Tag, err)
	}

	var insecureRegistries []string
	if bundleInfo.BundlePullOptions.InsecureRegistry {
		reg := reference.Domain(ref)
		insecureRegistries = append(insecureRegistries, reg)
	}

	bundle, _, err := remotes.Pull(context.Background(), ref, remotes.CreateResolver(config.LoadDefaultConfigFile(os.Stderr), insecureRegistries...))
	if err != nil {
		return fmt.Errorf("Unable to pull remote bundle %w", err)
	}
	bundleInfo.RPBundle = bundle
	return nil
}

func validateBundleTag(tag string) (reference.Named, error) {
	ref, err := reference.ParseNormalizedNamed(tag)
	log.Debugf("Attempting to validate bundle tag  %s", tag)
	if err != nil {
		return nil, fmt.Errorf("Invalid bundle tag format %s, expected REGISTRY/name:tag %w", tag, err)
	}
	log.Debugf("Successfully validated bundle tag  %s", tag)
	return ref, nil
}
