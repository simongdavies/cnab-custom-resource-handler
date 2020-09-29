package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-04-01/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	az "github.com/simongdavies/cnab-custom-resource-handler/pkg/azure"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/handlers"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/helpers"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var settings = map[string]string{
	"StorageAccountName":   "AZURE_STORAGE_ACCOUNT",
	"StorageResourceGroup": "AZURE_STORAGE_RESOURCE_GROUP",
	"SusbcriptionId":       "AZURE_SUBSCRIPTION_ID",
}

const (
	AzureStorageConnectionString = "AZURE_STORAGE_CONNECTION_STRING"
)

var debug bool
var rootCmd = &cobra.Command{
	Use:   "cnabcustomrphandler",
	Short: "Launches a web server that provides ARM RPC compliant CRUD endpoints for a CNAB Bundle ",
	Long:  `Launches a web server that provides ARM RPC compliant CRUD endpoints for a CNAB Bundle which can be used as an ARM Custom resource provider implementation for CNAB `,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
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
		if err := setStorageAccountConnectionString(); err != nil {
			log.Errorf("Error setting connection string %v", err)
			return err
		}
		log.Debug("Creating Router")
		router := chi.NewRouter()
		router.Use(middleware.RequestID)
		router.Use(middleware.RealIP)
		router.Use(middleware.Logger)
		router.Use(middleware.Timeout(60 * time.Second))
		router.Use(middleware.Recoverer)
		log.Debug("Creating Handler")
		router.Handle("/", handlers.NewCustomResourceHandler())
		log.Infof("Starting to listen on port  %s", port)
		err := http.ListenAndServe(fmt.Sprintf(":%s", port), router)
		if err != nil {
			log.Errorf("Error running HTTP Server %v", err)
			return err
		}
		return nil
	},
}

func main() {
	rootCmd.Execute()
}

func init() {
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "specifies if debug output should be produced")
}

func loadSettings() error {
	for k, v := range settings {
		val := os.Getenv(v)
		if len(val) == 0 {
			return fmt.Errorf("Environment Variable %s is not set", v)
		}
		settings[k] = strings.TrimSpace(val)
	}
	return nil
}

// setStorageAccountConnectionString. The Azure plugin expects the connection string to be set in an environment variable tis function looks up the key for the storage account and sts the variabe
func setStorageAccountConnectionString() error {
	var loginInfo az.LoginInfo
	var err error
	if loginInfo, err = az.LoginToAzure(); err != nil {
		return fmt.Errorf("Login to Azure Failed: %v", err)
	}
	result, err := getstorageAccountKey(loginInfo.Authorizer, settings["SusbcriptionId"], settings["StorageResourceGroup"], settings["StorageAccountName"])
	if err != nil {
		return fmt.Errorf("Get Storage Account Key Failed: %v", err)
	}

	os.Setenv(AzureStorageConnectionString, fmt.Sprintf("AccountName=%s;AccountKey=%s", settings["StorageAccountName"], *(((*result.Keys)[0]).Value)))
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
