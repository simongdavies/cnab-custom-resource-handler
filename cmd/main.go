package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"get.porter.sh/porter/pkg/porter"
	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-04-01/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/docker/distribution/reference"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	az "github.com/simongdavies/cnab-custom-resource-handler/pkg/azure"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/common"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/handlers"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/helpers"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/jobs"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var requiredSettings = map[string]string{
	"StorageAccountName":   "CNAB_AZURE_STATE_STORAGE_ACCOUNT_NAME",
	"StorageResourceGroup": "CNAB_AZURE_STATE_STORAGE_RESOURCE_GROUP",
	"SusbcriptionId":       "CNAB_AZURE_SUBSCRIPTION_ID",
	"BundleTag":            "CNAB_BUNDLE_TAG",
	"AsyncOpTable":         "CUSTOM_RP_ASYNC_OP_TABLE",
	"StateTable":           "CUSTOM_RP_STATE_TABLE",
	"CustomRPType":         "CUSTOM_RP_TYPE",
}

var optionalSettings = map[string]interface{}{
	"AllowInsecureRegistry": "CNAB_BUNDLE_INSECURE_REGISTRY",
	"ForcePull":             "CNAB_BUNDLE_FORCE_PULL",
}

const (
	AzureStorageConnectionString = "AZURE_STORAGE_CONNECTION_STRING"
	CnabStateStorageAccountKey   = "CNAB_AZURE_STATE_STORAGE_ACCOUNT_KEY"
)

var debug bool
var rootCmd = &cobra.Command{
	Use:   "cnabcustomrphandler",
	Short: "Launches a web server that provides ARM RPC compliant CRUD endpoints for a CNAB Bundle ",
	Long:  `Launches a web server that provides ARM RPC compliant CRUD endpoints for a CNAB Bundle which can be used as an ARM Custom resource provider implementation for CNAB `,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		log.SetReportCaller(true)
		if debug {
			log.SetLevel(log.DebugLevel)
		}
		port, exists := os.LookupEnv("LISTENER_PORT")
		if !exists {
			port = "8080"
		}
		if err := loadSettings(); err != nil {
			log.Errorf("Error loading settings %v", err)
			return err
		}

		if err := setAzureStorageInfo(); err != nil {
			log.Errorf("Error setting connection string %v", err)
			return err
		}
		ref, err := validateBundleTag(requiredSettings["BundleTag"])
		if err != nil {
			log.Errorf("Error validating bundle tag %v", err)
			return err
		}
		az.RPType = requiredSettings["CustomRPType"]

		// TODO need to handle digests and versioning correctly
		if _, ok := ref.(reference.Tagged); !ok {
			if _, ok := ref.(reference.Digested); !ok {
				requiredSettings["BundleTag"] += ":latest"
			}
		}

		common.TrimmedBundleTag = reference.TrimNamed(ref).String()
		common.BundlePullOptions = &porter.BundlePullOptions{
			Tag:              requiredSettings["BundleTag"],
			Force:            optionalSettings["ForcePull"].(bool),
			InsecureRegistry: optionalSettings["AllowInsecureRegistry"].(bool),
		}
		bun, err := common.PullBundle()
		if err != nil {
			log.Errorf("Error pulling bundle %v", err)
			return err
		}
		common.RPBundle = bun

		jobs.Start()
		log.Debug("Creating Router")
		router := chi.NewRouter()
		router.Use(az.ValidateRPType)
		router.Use(az.Login)
		router.Use(middleware.RequestID)
		router.Use(middleware.RealIP)
		router.Use(middleware.Logger)
		router.Use(middleware.Timeout(10 * time.Minute))
		router.Use(middleware.Recoverer)
		log.Debug("Creating Handler")
		router.Handle("/*", handlers.NewCustomResourceHandler())
		log.Infof("Starting to listen on port  %s", port)
		err = http.ListenAndServe(fmt.Sprintf(":%s", port), router)
		if err != nil {
			log.Errorf("Error running HTTP Server %v", err)
			return err
		}
		jobs.Stop()
		return nil
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "specifies if debug output should be produced")
}

func loadSettings() error {
	for k, v := range requiredSettings {
		val := os.Getenv(v)
		if len(val) == 0 {
			return fmt.Errorf("Environment Variable %s is not set", v)
		}
		requiredSettings[k] = strings.TrimSpace(val)
	}
	for k, v := range optionalSettings {
		val := os.Getenv(v.(string))
		if boolVal, err := strconv.ParseBool(val); err != nil {
			optionalSettings[k] = boolVal
		} else {
			optionalSettings[k] = false
		}
	}
	return nil
}

// setAzureStorageInfo. The Azure plugin expects the connection string to be set in an environment variable , the Azure CNAB Driver requires an account key to access file shares and the storage package requires details of the storage account and tables that are used
func setAzureStorageInfo() error {
	var loginInfo az.LoginInfo
	var err error
	if loginInfo, err = az.LoginToAzure(); err != nil {
		return fmt.Errorf("Login to Azure Failed: %v", err)
	}
	result, err := getstorageAccountKey(loginInfo.Authorizer, requiredSettings["SusbcriptionId"], requiredSettings["StorageResourceGroup"], requiredSettings["StorageAccountName"])
	if err != nil {
		return fmt.Errorf("Get Storage Account Key Failed: %v", err)
	}

	os.Setenv(AzureStorageConnectionString, fmt.Sprintf("AccountName=%s;AccountKey=%s", requiredSettings["StorageAccountName"], *(((*result.Keys)[0]).Value)))
	// this is used by the Azure CNAB Driver
	os.Setenv(CnabStateStorageAccountKey, *(((*result.Keys)[0]).Value))
	// these are used by Table Storage functions
	az.StorageAccountName = requiredSettings["StorageAccountName"]
	az.StorageAccountKey = *(((*result.Keys)[0]).Value)
	az.StateTableName = requiredSettings["StateTable"]
	az.AsyncOperationTableName = requiredSettings["AsyncOpTable"]
	return nil
}

func getstorageAccountKey(authorizer autorest.Authorizer, subscriptionID string, resourceGroupName string, storageAccountName string) (*storage.AccountListKeysResult, error) {
	client := az.GetStorageAccountsClient(subscriptionID, authorizer, helpers.UserAgent())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log.Debugf("Attempting to get storage account key for storage account %s in resource group %s in subscription %s", storageAccountName, resourceGroupName, subscriptionID)
	result, err := client.ListKeys(ctx, resourceGroupName, storageAccountName, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get storage account keys: %s", err)
	}
	return &result, nil
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
