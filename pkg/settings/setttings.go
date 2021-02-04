package settings

import (
	"context"
	"errors"
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
	"github.com/spf13/viper"
)

var IsRPaaS bool
var LogRequestBody bool
var LogResponseBody bool
var Debug bool

var RequiredSettings = map[string]string{
	"StorageAccountName":   "CNAB_AZURE_STATE_STORAGE_ACCOUNT_NAME",
	"StorageResourceGroup": "CNAB_AZURE_STATE_STORAGE_RESOURCE_GROUP",
	"SusbcriptionId":       "CNAB_AZURE_SUBSCRIPTION_ID",
	"AsyncOpTable":         "CUSTOM_RP_ASYNC_OP_TABLE",
	"StateTable":           "CUSTOM_RP_STATE_TABLE",
}

var OptionalSettings = map[string]interface{}{
	"AllowInsecureRegistry": "CNAB_BUNDLE_INSECURE_REGISTRY:bool",
	"ForcePull":             "CNAB_BUNDLE_FORCE_PULL:bool",
	"LogRequestBody":        "LOG_REQUEST_BODY:bool",
	"LogResponseBody":       "LOG_RESPONSE_BODY:bool",
	"IsRPaaS":               "IS_RPAAS:bool",
	"ResourceType":          "RESOURCE_TYPE:string",
	"BundleTag":             "CNAB_BUNDLE_TAG:string",
}

type BundleInformation struct {
	ResourceType      string
	ResourceProvider  string
	BundlePullOptions *porter.BundlePullOptions
	TrimmedBundleTag  string
	RPBundle          *bundle.Bundle
}

type Mapping struct {
	Provider              string `mapstructure:"provider"`
	Type                  string `mapstructure:"type"`
	Tag                   string `mapstructure:"tag"`
	ForcePull             bool   `mapstructure:"forcepull"`
	AllowInsecureRegistry bool   `mapstructure:"insecureregistry"`
}

var RPToProvider = make(map[string]*BundleInformation)

type Config struct {
	Mappings []Mapping
}

var mappingConfiguration Config

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

	resourceTypeName := OptionalSettings["ResourceType"].(string)
	IsRPaaS = OptionalSettings["IsRPaaS"].(bool)
	if IsRPaaS {
		log.Debug("Running as RPaaS Endpoint")
		viper.SetConfigType("yaml")
		viper.SetConfigName("providermapping")
		viper.AddConfigPath("$HOME/.cnabrp")
		viper.AddConfigPath(".")
		err := viper.ReadInConfig()
		if err != nil {
			log.Errorf("Error reading config file: %v \n", err)
			return err
		}
		err = viper.Unmarshal(&mappingConfiguration)
		if err != nil {
			log.Errorf("Error decoding config file: %v \n", err)
			return err
		}

		for _, m := range mappingConfiguration.Mappings {
			log.Debugf("Processing Mapping for Provider %s Type %s Tag %s", m.Provider, m.Type, m.Tag)
			bundleInformation, err := getBundleInfo(m.Provider, m.Type, m.Tag, m.ForcePull, m.AllowInsecureRegistry)
			if err != nil {
				return err
			}
			rpType := GetRPName(m.Provider, m.Type)
			RPToProvider[rpType] = bundleInformation
		}

	} else {
		log.Debug("Running as CustomRP Endpoint")
		resourceProviderName := "Microsoft.CustomProviders"

		bundleTag, ok := OptionalSettings["BundleTag"].(string)
		if !ok || len(bundleTag) == 0 {
			return errors.New("Environment Variable CNAB_BUNDLE_TAG should be set when running as Custom RP ")
		}

		bundleInformation, err := getBundleInfo(resourceProviderName, resourceTypeName, bundleTag, OptionalSettings["ForcePull"].(bool), OptionalSettings["AllowInsecureRegistry"].(bool))
		if err != nil {
			return err
		}
		rpType := GetRPName(resourceProviderName, resourceTypeName)
		RPToProvider[rpType] = bundleInformation
		log.Debugf("Processing Requests for Type %s Tag %s", bundleInformation.ResourceType, bundleInformation.BundlePullOptions.Tag)
	}

	LogRequestBody = OptionalSettings["LogRequestBody"].(bool)
	LogResponseBody = OptionalSettings["LogResponseBody"].(bool)

	return nil
}

func getBundleInfo(resourceProviderName string, resourceTypeName string, bundleTag string, force bool, allowInsecureRegistry bool) (*BundleInformation, error) {

	bundleInformation := BundleInformation{
		ResourceProvider: resourceProviderName,
		ResourceType:     resourceTypeName,
	}

	ref, err := validateBundleTag(bundleTag)
	if err != nil {
		log.Errorf("Error validating bundle tag %v", err)
		return nil, err
	}
	// TODO need to handle digests and versioning correctly
	if _, ok := ref.(reference.Tagged); !ok {
		if _, ok := ref.(reference.Digested); !ok {
			bundleTag += ":latest"
		}
	}

	bundleInformation.TrimmedBundleTag = reference.TrimNamed(ref).String()
	log.Debugf("Trimmed Bundle Tag: %s", bundleInformation.TrimmedBundleTag)
	bundleInformation.BundlePullOptions = &porter.BundlePullOptions{
		Tag:              bundleTag,
		Force:            force,
		InsecureRegistry: allowInsecureRegistry,
	}

	if err := pullBundle(&bundleInformation); err != nil {
		log.Errorf("Error pulling bundle %v", err)
		return nil, err
	}
	return &bundleInformation, nil

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
