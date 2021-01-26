package azure

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-04-01/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/helpers"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/settings"
	log "github.com/sirupsen/logrus"
)

const (
	msiTokenEndpoint = "http://169.254.169.254/metadata/identity/oauth2/token"
)

// LoginInfo contains Azure login information
type LoginInfo struct {
	Authorizer         autorest.Authorizer
	OAuthTokenProvider adal.OAuthTokenProvider
}

func LoginToAzure() (LoginInfo, error) {
	var loginInfo LoginInfo
	var err error

	if checkForMSIEndpoint() {
		// Attempt to login with MSI
		log.Debug("Attempting to Login with MSI")
		msiConfig := auth.NewMSIConfig()
		loginInfo.Authorizer, err = msiConfig.Authorizer()
		if err == nil {
			log.Debug("Logged in with MSI")
			return loginInfo, nil
		}
	} else {
		log.Debug("Unable to find MSI Endpoint")
	}

	// Attempt to Login using azure CLI
	log.Debug("Attempting to Login with az cli")
	loginInfo.Authorizer, err = auth.NewAuthorizerFromCLI()
	if err == nil {
		log.Debug("Logged in with CLI")
		return loginInfo, nil
	} else {
		log.Debugf("Failed to login with Azure cli: %v", err)
	}

	return loginInfo, fmt.Errorf("Failed to login with MSI: %v", err)

}

func checkForMSIEndpoint() bool {
	var err error
	for i := 1; i < 4; i++ {
		timeout := time.Duration(time.Duration(i) * time.Second)
		client := http.Client{
			Timeout: timeout,
		}
		_, err = client.Head(msiTokenEndpoint)
		if err != nil {
			log.Debugf("Failed to get MSI endpoint:%v", err)
		} else {
			break
		}
	}
	return err == nil
}

// SetAzureStorageInfo. The Azure plugin expects the connection string to be set in an environment variable , the Azure CNAB Driver requires an account key to access file shares and the storage package requires details of the storage account and tables that are used
func SetAzureStorageInfo() error {
	var loginInfo LoginInfo
	var err error
	if loginInfo, err = LoginToAzure(); err != nil {
		return fmt.Errorf("Login to Azure Failed: %v", err)
	}
	result, err := getstorageAccountKey(loginInfo.Authorizer, settings.RequiredSettings["SusbcriptionId"], settings.RequiredSettings["StorageResourceGroup"], settings.RequiredSettings["StorageAccountName"])
	if err != nil {
		return fmt.Errorf("Get Storage Account Key Failed: %v", err)
	}

	os.Setenv(AzureStorageConnectionString, fmt.Sprintf("AccountName=%s;AccountKey=%s", settings.RequiredSettings["StorageAccountName"], *(((*result.Keys)[0]).Value)))
	// this is used by the Azure CNAB Driver
	os.Setenv(CnabStateStorageAccountKey, *(((*result.Keys)[0]).Value))
	// these are used by Table Storage functions
	StorageAccountName = settings.RequiredSettings["StorageAccountName"]
	StorageAccountKey = *(((*result.Keys)[0]).Value)
	StateTableName = settings.RequiredSettings["StateTable"]
	AsyncOperationTableName = settings.RequiredSettings["AsyncOpTable"]
	return nil
}

func getstorageAccountKey(authorizer autorest.Authorizer, subscriptionID string, resourceGroupName string, storageAccountName string) (*storage.AccountListKeysResult, error) {
	client := GetStorageAccountsClient(subscriptionID, authorizer, helpers.UserAgent())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log.Debugf("Attempting to get storage account key for storage account %s in resource group %s in subscription %s", storageAccountName, resourceGroupName, subscriptionID)
	result, err := client.ListKeys(ctx, resourceGroupName, storageAccountName, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get storage account keys: %s", err)
	}
	return &result, nil
}
