package azure

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure/auth"
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

	// Attempt to login with MSI
	if checkForMSIEndpoint() {
		log.Debug("Attempting to Login with MSI")
		msiConfig := auth.NewMSIConfig()
		loginInfo.Authorizer, err = msiConfig.Authorizer()
		if err != nil {
			return loginInfo, fmt.Errorf("Attempt to set Authorizer with MSI failed: %v", err)
		}
		log.Debug("Logged in with MSI")
		return loginInfo, nil
	}

	// Attempt to Login using azure CLI
	log.Debug("Attempting to Login with az cli")
	loginInfo.Authorizer, err = auth.NewAuthorizerFromCLI()
	if err == nil {
		log.Debug("Logged in with CLI")
		return loginInfo, nil
	}
	return loginInfo, fmt.Errorf("Cannot login to Azure - no valid credentials provided or available, failed to login with Azure cli: %v", err)
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
			break
		}

	}
	return err == nil
}
