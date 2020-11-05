package helpers

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/common"
	log "github.com/sirupsen/logrus"
)

// GetResourceDetails parses the request header containing resource details and returns the details of the resource
func GetResourceDetails(r *http.Request) (*azure.Resource, *string, *string, error) {
	requestPath := r.URL.Path
	resource, err := azure.ParseResourceID(requestPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Failed to parse resource id from request Path: %v", err)
	}

	log.Debugf("Request path header: %s", requestPath)

	// if this is a POST the final node in the URL is the action name
	resourceId := requestPath
	if r.Method == "POST" {
		resourceId = strings.TrimSuffix(requestPath, fmt.Sprintf("/%s", resource.ResourceName))
	}
	log.Debugf("Resource Id: %s", resourceId)
	requestParts := strings.Split(resourceId, "/")
	resource.ResourceType = strings.Join(requestParts[6:len(requestParts)-1], "/")
	log.Debugf("Subscription Id: %s", resource.SubscriptionID)
	log.Debugf("Provider: %s", resource.Provider)
	log.Debugf("Resource Group: %s", resource.ResourceGroup)
	log.Debugf("Resource Name: %s", resource.ResourceName)
	log.Debugf("Resource Type: %s", resource.ResourceType)
	return &resource, &resourceId, &requestPath, nil
}
func GetInstallationName(requestPath string) string {
	data := []byte(fmt.Sprintf("%s%s", strings.ToLower(common.TrimmedBundleTag), strings.ToLower(requestPath)))
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}
