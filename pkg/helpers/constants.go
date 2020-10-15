package helpers

import (
	"fmt"

	"github.com/simongdavies/cnab-custom-resource-handler/pkg"
)

const (
	UserAgentPrefix            = "cnab-custom-resource-handler"
	ProvisioningStateSucceeded = "Succeeded"
	ProvisioningStateFailed    = "Failed"
	ProvisioningStateDeleting  = "Deleting"
	StatusSucceeded            = "Succeeded"
	StatusFailed               = "Failed"
	APIVersion                 = "2018-09-01-preview"
	AsyncOperationComplete     = "Succeeded"
	AsyncOperationFailed       = "Failed"
	AsyncOperationUnknown      = "Unknown"
)

// Version returns the version string
func Version() string {
	return fmt.Sprintf("%v-%v", pkg.Version, pkg.Commit)
}

func UserAgent() string {
	return fmt.Sprintf("%s-%s-%s", UserAgentPrefix, pkg.Version, pkg.Commit)
}
