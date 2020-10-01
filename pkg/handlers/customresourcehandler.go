package handlers

import (
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
	cnabRPData := r.Context().Value(models.BundleContext).(*models.BundleRP)
	log.Infof("Received Request: %s", cnabRPData.Properties.RequestPath)
	parts := strings.Split(cnabRPData.Properties.RequestPath, "/")
	installationName := parts[len(parts)-1]
	var args []string

	args = append(args, "install", installationName, "-t", cnabRPData.Properties.BundlePullOptions.Tag)

	if len(cnabRPData.Properties.Parameters) > 0 {
		paramFile, err := writeParametersFile(cnabRPData.Properties.Parameters)
		if err != nil {
			render.Render(w, r, helpers.ErrorInternalServerErrorFromError(err))
			return
		}
		args = append(args, "-p", paramFile.Name())
		defer os.Remove(paramFile.Name())
	}

	if len(cnabRPData.Properties.Credentials) > 0 {
		credFile, err := writeCredentialsFile(cnabRPData.Properties.Parameters)
		if err != nil {
			render.Render(w, r, helpers.ErrorInternalServerErrorFromError(err))
			return
		}
		args = append(args, "-c", credFile.Name())
		defer os.Remove(credFile.Name())
	}

	if out, err := executePorterCommand(args); err != nil {
		render.Render(w, r, helpers.ErrorInternalServerError(string(out)))
		return
	}

	args = []string{}
	args = append(args, "installations", "output", "list", "-i", installationName)
	out, err := executePorterCommand(args)
	if err != nil {
		render.Render(w, r, helpers.ErrorInternalServerError(string(out)))
		return
	}

	var cmdOutput []porterOutput
	if err := json.Unmarshal(out, cmdOutput); err != nil {
		render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to read json from command: %v", err)))
		return
	}

	output := models.BundleCommandOutputs{
		Outputs: make(map[string]interface{}),
	}
	for _, v := range cmdOutput {
		output.Outputs[v.Name] = v.Value
	}

	render.DefaultResponder(w, r, output)
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

func deleteCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Received Request: %s", r.RequestURI)
	render.DefaultResponder(w, r, fmt.Sprintf("Delete Hello World!! from %s", r.RequestURI))
}

func executePorterCommand(args []string) ([]byte, error) {
	args = append(args, "-d", "azure", "-o", "json")
	out, err := exec.Command("porter", args...).CombinedOutput()

	if err != nil {
		return nil, fmt.Errorf("Porter command failed: %v", err)
	}
	return out, nil
}
