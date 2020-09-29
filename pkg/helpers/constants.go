package helpers

import (
	"fmt"

	"github.com/simongdavies/cnab-custom-resource-handler/pkg"
)

const (
	UserAgentPrefix = "cnab-custom-resource-handler"
)

// Version returns the version string
func Version() string {
	return fmt.Sprintf("%v-%v", pkg.Version, pkg.Commit)
}

func UserAgent() string {
	return fmt.Sprintf("%s-%s-%s", UserAgentPrefix, pkg.Version, pkg.Commit)
}
