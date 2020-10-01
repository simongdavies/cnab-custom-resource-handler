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

	args = append(args, action, installationName, "-t", rpInput.Properties.BundlePullOptions.Tag)

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
		output.Outputs[v.Name] = v.Value
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

	ps := parameters.NewParameterSet("parameter-set")
	//TODO validate the parameters against the bundle
	for k, v := range params {
		name := getEnvVarName(k)
		val := fmt.Sprintf("%v", v)
		p := valuesource.Strategy{Name: name}
		p.Source.Key = host.SourceEnv
		p.Source.Value = val
		ps.Parameters = append(ps.Parameters, p)
		os.Setenv(name, val)
	}

	return writeFile(ps)
}

func writeCredentialsFile(creds map[string]interface{}) (*os.File, error) {

	cs := credentials.NewCredentialSet("credential-set")
	//TODO validate the credentials against the bundle
	for k, v := range creds {
		name := getEnvVarName(k)
		val := fmt.Sprintf("%v", v)
		c := valuesource.Strategy{Name: name}
		c.Source.Key = host.SourceEnv
		c.Source.Value = val
		cs.Credentials = append(cs.Credentials, c)
		os.Setenv(name, val)
	}

	return writeFile(cs)
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
	args = append(args, "--driver", "azure", "--output", "json")
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
