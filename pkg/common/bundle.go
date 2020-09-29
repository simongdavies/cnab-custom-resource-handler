package common

import (
	"get.porter.sh/porter/pkg/porter"
)

// BundleCommandProperties defines the bundle and the properties to be used for the command
type BundleCommandProperties struct {
	Credentials map[string]string `json:"credentials"`
	Parameters  map[string]string `json:"parameters"`
	*porter.BundlePullOptions
}
