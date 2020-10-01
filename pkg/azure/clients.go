package azure

import (
	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-04-01/storage"
	"github.com/Azure/go-autorest/autorest"
)

// GetStorageAccountsClient gets a Storage Account Client
func GetStorageAccountsClient(subscriptionID string, authorizer autorest.Authorizer, userAgent string) storage.AccountsClient {
	accountsClient := storage.NewAccountsClient(subscriptionID)
	accountsClient.Authorizer = authorizer
	_ = accountsClient.AddToUserAgent(userAgent)
	return accountsClient
}
