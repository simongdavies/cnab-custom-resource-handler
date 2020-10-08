package common

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"get.porter.sh/porter/pkg/parameters"
	"get.porter.sh/porter/pkg/porter"

	"github.com/cnabio/cnab-go/bundle"
	"github.com/cnabio/cnab-go/credentials"
	"github.com/cnabio/cnab-go/secrets/host"
	"github.com/cnabio/cnab-go/valuesource"
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

func WriteParametersFile(params map[string]interface{}, dir string) (*os.File, error) {

	ps := parameters.NewParameterSet("parameter-set")
	for k, v := range params {
		vs, err := setupArg(k, v, len(RPBundle.Parameters[k].Destination.Path) > 0, dir)
		if err != nil {
			return nil, fmt.Errorf("Failed to set up parameter: %v", err)
		}
		ps.Parameters = append(ps.Parameters, *vs)
	}

	return writeFile(ps)
}

func writeFile(filedata interface{}) (*os.File, error) {
	file, err := ioutil.TempFile("", "cnab*")
	if err != nil {
		return nil, fmt.Errorf("Failed to create temp file:%v", err)
	}

	data, err := json.Marshal(filedata)
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal data to json:%v", err)
	}

	_, err = file.Write(data)
	if err != nil {
		return nil, fmt.Errorf("Failed to write json to file %s error:%v", file.Name(), err)
	}

	return file, nil
}

func setupArg(key string, value interface{}, isFile bool, dir string) (*valuesource.Strategy, error) {
	name := getEnvVarName(key)
	val := fmt.Sprintf("%v", value)
	c := valuesource.Strategy{Name: key}

	if isFile {
		// File data should be encoded as base64
		file, err := ioutil.TempFile(dir, "cnab*")
		if err != nil {
			return nil, fmt.Errorf("Failed to create temp file for %s :%v", key, err)
		}
		c.Source.Key = host.SourcePath
		data, err := base64.StdEncoding.DecodeString(val)
		if err != nil {
			return nil, fmt.Errorf("Failed to decode data for %s :%v", key, err)
		}
		if _, err := file.Write(data); err != nil {
			return nil, fmt.Errorf("Failed to write date to file for %s :%v", key, err)
		}
		c.Source.Value = file.Name()
	} else {
		c.Source.Key = host.SourceEnv
		c.Source.Value = name
		os.Setenv(name, val)
	}
	return &c, nil
}

func WriteCredentialsFile(creds map[string]interface{}, dir string) (*os.File, error) {

	cs := credentials.NewCredentialSet("credential-set")
	for k, v := range creds {
		vs, err := setupArg(k, v, len(RPBundle.Credentials[k].Path) > 0, dir)
		if err != nil {
			return nil, fmt.Errorf("Failed to set up credential: %v", err)
		}
		cs.Credentials = append(cs.Credentials, *vs)
	}

	return writeFile(cs)
}

func getEnvVarName(name string) string {
	return strings.ToUpper(strings.Replace(name, "-", "_", -1))
}
