package common

import (
	"context"
	"fmt"
	"os"

	"get.porter.sh/porter/pkg/porter"
	"github.com/cnabio/cnab-go/bundle"
	"github.com/cnabio/cnab-to-oci/remotes"
	"github.com/docker/cli/cli/config"
	"github.com/docker/distribution/reference"
)

var BundlePullOptions *porter.BundlePullOptions
var TrimmedBundleTag string
var RPBundle *bundle.Bundle

func PullBundle() (*bundle.Bundle, error) {
	ref, err := reference.ParseNormalizedNamed(BundlePullOptions.Tag)
	if err != nil {
		return nil, fmt.Errorf("Invalid bundle tag format %s, expected REGISTRY/name:tag %w", BundlePullOptions.Tag, err)
	}

	var insecureRegistries []string
	if BundlePullOptions.InsecureRegistry {
		reg := reference.Domain(ref)
		insecureRegistries = append(insecureRegistries, reg)
	}

	bundle, _, err := remotes.Pull(context.Background(), ref, remotes.CreateResolver(config.LoadDefaultConfigFile(os.Stderr), insecureRegistries...))
	if err != nil {
		return nil, fmt.Errorf("Unable to pull remote bundle %w", err)
	}
	return bundle, nil
}
