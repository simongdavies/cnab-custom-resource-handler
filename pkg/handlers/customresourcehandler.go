package handlers

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	az "github.com/Azure/go-autorest/autorest/azure"
	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/azure"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/common"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/helpers"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/jobs"
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
	r.Use(azure.LoadState)
	r.Use(models.BundleCtx)
	r.Get("/*", getCustomResourceHandler)
	r.Put("/*", putCustomResourceHandler)
	r.Post("/*", postCustomResourceHandler)
	r.Delete("/*", deleteCustomResourceHandler)
	return r
}

func getCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	rpInput := r.Context().Value(models.BundleContext).(*models.BundleRP)
	log.Infof("Received GET Request: %s", rpInput.Id)

	if azure.IsOperationsRequest(rpInput.Id) {
		getOperationHandler(w, r)
		return
	}

	// TODO change to use Custom RP Type name
	if rpInput.Name == "installs" {
		listCustomResourceHandler(w, r, rpInput.SubscriptionId)
		return
	}

	installationName := getInstallationName(rpInput.Id)

	// The last attempt to update the resource failed

	if rpInput.Properties.ProvisioningState == helpers.ProvisioningStateFailed && rpInput.Properties.ErrorResponse != nil {
		output := make(map[string]interface{})
		output["ProvisioningState"] = rpInput.Properties.ProvisioningState
		output["Error"] = rpInput.Properties.ErrorResponse.RequestError.Message
		rpOutput := models.BundleRPOutput{
			RPProperties: &models.RPProperties{
				Type: rpInput.Type,
				Id:   rpInput.Id,
				Name: rpInput.Name,
			},
			BundleCommandOutputs: &models.BundleCommandOutputs{
				Outputs: output,
			},
		}
		_ = render.Render(w, r, &rpOutput)
		return
	}

	// The resource is not in a terminal state

	if azure.IsTerminalProvisioningState(rpInput.Properties.ProvisioningState) {
		if exists, err := checkIfInstallationExists(installationName); err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to check for existing installation: %v", err)))
			return
		} else if !exists {
			// This can only happen if the installation was deleted outside of the RP
			// TODO clean up state
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}

	rpOutput, err := getRPOutput(installationName, rpInput, rpInput.Properties.ProvisioningState)
	if err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed getRPOutput: %v", err)))
		return
	}
	render.DefaultResponder(w, r, rpOutput)
}

func listCustomResourceHandler(w http.ResponseWriter, r *http.Request, subscriptionId string) {
	// TODO handle paging
	res, err := azure.ListRPState(subscriptionId)
	if err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed getRPOutput: %v", err)))
		return
	}

	list := make([]*models.BundleRPOutput, 0)
	for _, v := range res.Entities {
		id := azure.GetResourceIdFromRowKey(v.RowKey)
		resource, err := az.ParseResourceID(id)
		if err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to parse resourceId %s: %v", v.RowKey, err)))
			return
		}
		requestParts := strings.Split(id, "/")
		output := models.BundleRPOutput{
			RPProperties: &models.RPProperties{
				Type: strings.Join(requestParts[6:len(requestParts)-1], "/"),
				Id:   id,
				Name: resource.ResourceName,
			},
		}
		list = append(list, &output)
	}
	render.DefaultResponder(w, r, list)
}

func putCustomResourceHandler(w http.ResponseWriter, r *http.Request) {

	rpInput := r.Context().Value(models.BundleContext).(*models.BundleRP)
	log.Infof("Received PUT Request: %s", rpInput.Id)
	installationName := getInstallationName(rpInput.Id)

	action := "install"
	provisioningState := "Installing"
	if exists, err := checkIfInstallationExists(installationName); err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to check for existing installation: %v", err)))
		return
	} else if exists {
		action = "upgrade"
		provisioningState = "Upgrading"
	}

	var args []string
	args = append(args, action, installationName, "--tag", rpInput.Properties.BundlePullOptions.Tag)

	if len(rpInput.Properties.Parameters) > 0 {
		if err := validateParameters(rpInput.Properties.Parameters); err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to validate parameters:%v", err)))
			return
		}
	}

	if len(rpInput.Properties.Credentials) > 0 {
		if err := validateCredentials(rpInput.Properties.Credentials); err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to validate credentials:%v", err)))
			return
		}
	}

	jobData := jobs.PutJobData{
		RPInput:          rpInput,
		Args:             args,
		InstallationName: installationName,
	}

	jobs.PutJobs <- &jobData

	rpInput.Properties.ProvisioningState = provisioningState
	if err := azure.PutRPState(rpInput.SubscriptionId, rpInput.Id, rpInput.Properties); err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to update state:%v", err)))
		return
	}

	rpOutput, err := getRPOutput(installationName, rpInput, provisioningState)
	if err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to get RP output:%v", err)))
		return
	}

	status := http.StatusCreated
	if action == "upgrade" {
		status = http.StatusOK
	}

	render.Status(r, status)
	render.DefaultResponder(w, r, rpOutput)

}

func getRPOutput(installationName string, rpInput *models.BundleRP, provisioningState string) (*models.BundleRPOutput, error) {

	var cmdOutput []porterOutput

	if provisioningState == helpers.ProvisioningStateSucceeded {
		args := []string{}
		args = append(args, "installations", "output", "list", "-i", installationName)
		out, err := helpers.ExecutePorterCommand(args)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(out, &cmdOutput); err != nil {
			return nil, fmt.Errorf("Failed to read json from command: %v", err)
		}
	}

	output := make(map[string]interface{})

	output["ProvisioningState"] = provisioningState
	output["Installation"] = installationName
	for _, v := range cmdOutput {
		log.Debugf("Installation Name:%s Output:%s", installationName, v.Name)
		if IsSenstive, _ := common.RPBundle.IsOutputSensitive(v.Name); !IsSenstive {
			output[v.Name] = strings.TrimSuffix(v.Value, "\\n")
		}
	}

	for k, v := range rpInput.Properties.Parameters {
		output[k] = v
	}

	rpOutput := models.BundleRPOutput{
		RPProperties: &models.RPProperties{
			Type: rpInput.Type,
			Id:   rpInput.Id,
			Name: rpInput.Name,
		},
		BundleCommandOutputs: &models.BundleCommandOutputs{
			Outputs: output,
		},
	}
	return &rpOutput, nil
}

func validateCredentials(creds map[string]interface{}) error {

	for k := range common.RPBundle.Credentials {
		log.Debugf("Credential Name:%s", k)
	}

	for k, v := range common.RPBundle.Credentials {
		if _, ok := creds[k]; !ok && v.Required {
			return fmt.Errorf("Credential %s is required", k)
		}
	}

	for k := range creds {
		if _, ok := common.RPBundle.Credentials[k]; !ok {
			return fmt.Errorf("Credential %s is not specified in bundle", k)
		}
	}
	return nil
}

func validateParameters(params map[string]interface{}) error {

	for k := range common.RPBundle.Parameters {
		log.Debugf("Parameter Name:%s", k)
	}

	for k, v := range common.RPBundle.Parameters {
		if _, ok := params[k]; !ok && v.Required {
			return fmt.Errorf("Parameter %s is required", k)
		}
	}

	for k := range params {
		if _, ok := common.RPBundle.Parameters[k]; !ok {
			return fmt.Errorf("Parameter %s is not specified in bundle", k)
		}
	}
	return nil
}

func postCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Received Request: %s", r.RequestURI)
	render.DefaultResponder(w, r, fmt.Sprintf("Post Hello World!! from %s", r.RequestURI))
}

func deleteCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	rpInput := r.Context().Value(models.BundleContext).(*models.BundleRP)
	guid := rpInput.Properties.OperationId
	if rpInput.Properties.ProvisioningState != helpers.ProvisioningStateDeleting {

		installationName := getInstallationName(rpInput.Id)
		var args []string
		if exists, err := checkIfInstallationExists(installationName); err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to check for existing installation: %v", err)))
			return
		} else if !exists {
			// This should only happen if the resource is deleted outside of ARM
			// TODO clean-up state
			w.WriteHeader(http.StatusNoContent)
			return
		}

		guid = uuid.New().String()
		rpInput.Properties.ProvisioningState = helpers.ProvisioningStateDeleting
		rpInput.Properties.OperationId = guid
		jobData := jobs.DeleteJobData{
			RPInput:          rpInput,
			Args:             args,
			InstallationName: installationName,
			OperationId:      guid,
		}

		jobs.DeleteJobs <- &jobData

		if err := azure.PutRPState(rpInput.SubscriptionId, rpInput.Id, rpInput.Properties); err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to update state:%v", err)))
			return
		}
		if err := azure.PutAsyncOp(rpInput.SubscriptionId, guid, "delete", helpers.ProvisioningStateDeleting); err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to update asyncop %s :%v", guid, err)))
			return
		}

	}

	w.Header().Add("Retry-After", "60")
	w.Header().Add("Location", fmt.Sprintf("https://management.azure.com%s/operations/%s&api-version=%s", rpInput.Id, guid, helpers.APIVersion))
	w.WriteHeader(http.StatusAccepted)

}

func getOperationHandler(w http.ResponseWriter, r *http.Request) {

	// TODO validate the operation Id

	rpInput := r.Context().Value(models.BundleContext).(*models.BundleRP)

	state, err := azure.GetAsyncOp(rpInput.SubscriptionId, rpInput.Name)
	if err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to get async op %s :%v", rpInput.Name, err)))
		return
	}
	if state.Action == "delete" && state.Status != helpers.ProvisioningStateDeleting {
		_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Unexpected status for delete action %s :%v", rpInput.Name, err)))
		return
	}
	if state.Status == helpers.AsyncOperationComplete {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Add("Retry-After", "60")
	w.Header().Add("Location", fmt.Sprintf("https://management.azure.com%s&api-version=%s", rpInput.Id, helpers.APIVersion))
	w.WriteHeader(http.StatusAccepted)

}

func getInstallationName(requestPath string) string {
	data := []byte(fmt.Sprintf("%s%s", strings.ToLower(common.TrimmedBundleTag), strings.ToLower(requestPath)))
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

//TODO handle failed/successful installs
func checkIfInstallationExists(name string) (bool, error) {
	args := []string{}
	args = append(args, "installations", "show", name)
	if out, err := helpers.ExecutePorterCommand(args); err != nil {
		if strings.Contains(strings.ToLower(string(out)), "installation does not exist") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
