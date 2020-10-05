package handlers

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"get.porter.sh/porter/pkg/parameters"
	"github.com/cnabio/cnab-go/credentials"
	"github.com/cnabio/cnab-go/secrets/host"
	"github.com/cnabio/cnab-go/valuesource"
	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/azure"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/common"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/helpers"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/models"
	log "github.com/sirupsen/logrus"
)

type porterOutput struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
	Type  string `json:"Type"`
}

func NewCustomResourceHandler() chi.Router {
	r := chi.NewRouter()
	r.Use(render.SetContentType(render.ContentTypeJSON))
	r.Use(azure.Login)
	r.Use(models.BundleCtx)
	r.Get("/*", getCustomResourceHandler)
	r.Put("/*", putCustomResourceHandler)
	r.Post("/*", postCustomResourceHandler)
	r.Delete("/*", deleteCustomResourceHandler)
	return r
}

func getCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	requestPath := r.Header.Get("x-ms-customproviders-requestpath")
	log.Infof("Received Request: %s", requestPath)
	render.DefaultResponder(w, r, fmt.Sprintf("Get Hello World!! from %s", r.URL.String()))
}

func putCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	rpInput := r.Context().Value(models.BundleContext).(*models.BundleRP)
	log.Infof("Received PUT Request: %s", rpInput.Properties.RequestPath)
	installationName := getInstallationName(rpInput.Properties.RequestPath)
	var args []string

	action := "install"
	if exists, err := checkIfInstallationExists(installationName); err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to check for existing installation: %v", err)))
		return
	} else if exists {
		action = "upgrade"
	}

	args = append(args, action, installationName, "--tag", rpInput.Properties.BundlePullOptions.Tag)

	if len(rpInput.Properties.Parameters) > 0 {
		paramFile, err := writeParametersFile(rpInput.Properties.Parameters)
		if err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(err))
			return
		}
		args = append(args, "-p", paramFile.Name())
		defer os.Remove(paramFile.Name())
	}

	if len(rpInput.Properties.Credentials) > 0 {
		credFile, err := writeCredentialsFile(rpInput.Properties.Parameters)
		if err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(err))
			return
		}
		args = append(args, "-c", credFile.Name())
		defer os.Remove(credFile.Name())
	}

	if out, err := executePorterCommand(args); err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerError(string(out)))
		return
	}

	args = []string{}
	args = append(args, "installations", "output", "list", "-i", installationName)
	out, err := executePorterCommand(args)
	if err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerError(string(out)))
		return
	}

	var cmdOutput []porterOutput
	if err := json.Unmarshal(out, &cmdOutput); err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to read json from command: %v", err)))
		return
	}

	output := models.BundleCommandOutputs{
		Outputs: make(map[string]interface{}),
	}
	for _, v := range cmdOutput {
		if IsSenstive, _ := common.Bundle.IsOutputSensitive(v.Name); !IsSenstive {
			output.Outputs[v.Name] = v.Value
		}
	}

	rpOutput := models.BundleRPOutput{
		RPProperties: &models.RPProperties{
			Type:         rpInput.Type,
			Id:           rpInput.Id,
			Name:         rpInput.Name,
			Installation: installationName,
		},
		Properties: &output,
	}
	render.DefaultResponder(w, r, rpOutput)
}

func writeParametersFile(params map[string]interface{}) (*os.File, error) {

	if err := validateParameters(params); err != nil {
		return nil, err
	}

	ps := parameters.NewParameterSet("parameter-set")
	for k, v := range params {
		vs, err := setupArg(k, v)
		if err != nil {
			return nil, fmt.Errorf("Failed to set up parameter: %v", err)
		}
		ps.Parameters = append(ps.Parameters, *vs)
	}

	return writeFile(ps)
}

func writeCredentialsFile(creds map[string]interface{}) (*os.File, error) {

	if err := validateCredentials(creds); err != nil {
		return nil, err
	}

	cs := credentials.NewCredentialSet("credential-set")
	for k, v := range creds {
		vs, err := setupArg(k, v)
		if err != nil {
			return nil, fmt.Errorf("Failed to set up credential: %v", err)
		}
		cs.Credentials = append(cs.Credentials, *vs)
	}

	return writeFile(cs)
}

func validateCredentials(creds map[string]interface{}) error {

	for k := range common.Bundle.Credentials {
		log.Debugf("Credential Name:%", k)
	}

	for k, v := range common.Bundle.Credentials {
		if _, ok := creds[k]; !ok && v.Required {
			return fmt.Errorf("Credential %s is required", k)
		}
	}

	for k := range creds {
		if _, ok := common.Bundle.Credentials[k]; !ok {
			return fmt.Errorf("Credential %s is not specified in bundle", k)
		}
	}
	return nil
}

func validateParameters(params map[string]interface{}) error {

	for k := range common.Bundle.Parameters {
		log.Debugf("Parameter Name:%", k)
	}

	for k, v := range common.Bundle.Parameters {
		if _, ok := params[k]; !ok && v.Required {
			return fmt.Errorf("Parameter %s is required", k)
		}
	}

	for k := range params {
		if _, ok := common.Bundle.Parameters[k]; !ok {
			return fmt.Errorf("Parameter %s is not specified in bundle", k)
		}
	}
	return nil
}

func setupArg(key string, value interface{}) (*valuesource.Strategy, error) {
	name := getEnvVarName(key)
	val := fmt.Sprintf("%v", value)
	c := valuesource.Strategy{Name: key}
	c.Source.Key = host.SourceEnv
	c.Source.Value = name
	os.Setenv(name, val)
	return &c, nil
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

func getEnvVarName(name string) string {
	return strings.ToUpper(strings.Replace(name, "-", "_", -1))
}

func postCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Received Request: %s", r.RequestURI)
	render.DefaultResponder(w, r, fmt.Sprintf("Post Hello World!! from %s", r.RequestURI))
}

//TODO handle async operations
func deleteCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	rpInput := r.Context().Value(models.BundleContext).(*models.BundleRP)
	log.Infof("Received DELETE Request: %s", rpInput.Properties.RequestPath)
	installationName := getInstallationName(rpInput.Properties.RequestPath)
	var args []string

	if exists, err := checkIfInstallationExists(installationName); err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to check for existing installation: %v", err)))
		return
	} else if !exists {
		render.Status(r, http.StatusNoContent)
		return
	}

	args = append(args, "uninstall", installationName, "--delete")

	out, err := executePorterCommand(args)
	if err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerError(string(out)))
		return
	}
	render.Status(r, http.StatusOK)
}

func executePorterCommand(args []string) ([]byte, error) {
	if isDriverCommand(args[0]) {
		args = append(args, "--driver", "azure")
	}

	if isOutputCommand(args[0]) {
		args = append(args, "--output", "json")
	}

	log.Debugf("porter %v", args)
	out, err := exec.Command("porter", args...).CombinedOutput()
	if err != nil {
		log.Debugf("Command failed Error:%v Output: %s", err, string(out))
		return out, fmt.Errorf("Porter command failed: %v", err)
	}

	return out, nil
}

func getInstallationName(requestPath string) string {
	data := []byte(fmt.Sprintf("%s%s", strings.ToLower(common.TrimmedBundleTag), strings.ToLower(requestPath)))
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

func checkIfInstallationExists(name string) (bool, error) {
	args := []string{}
	args = append(args, "installations", "show", name)
	if out, err := executePorterCommand(args); err != nil {
		if strings.Contains(strings.ToLower(string(out)), "installation does not exist") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func isDriverCommand(cmd string) bool {
	return strings.Contains("installupgradeuninstallaction", cmd)
}

func isOutputCommand(cmd string) bool {
	return cmd == "installations"
}
