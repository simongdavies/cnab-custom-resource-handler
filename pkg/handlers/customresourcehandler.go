package handlers

import (
	"fmt"
	"net/http"
	"os"
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
	log.Infof("Received GET Request: %s", rpInput.RequestPath)

	if azure.IsOperationsRequest(rpInput.Id) {
		getOperationHandler(w, r)
		return
	}

	// TODO change to use Custom RP Type name
	if rpInput.Name == "installs" {
		listCustomResourceHandler(w, r, rpInput.SubscriptionId)
		return
	}

	installationName := helpers.GetInstallationName(rpInput.Id)

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
	log.Infof("Received PUT Request: %s", rpInput.RequestPath)
	installationName := helpers.GetInstallationName(rpInput.Id)

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
		if err := validateParameters(rpInput.Properties.Parameters, action); err != nil {
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

	var cmdOutput []helpers.PorterOutput
	var err error

	if provisioningState == helpers.ProvisioningStateSucceeded {
		cmdOutput, err = helpers.GetBundleOutput(installationName, []string{"install", "upgrade"})
		if err != nil {
			return nil, err
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

func validateParameters(params map[string]interface{}, action string) error {

	for k := range common.RPBundle.Parameters {
		log.Debugf("Parameter Name:%s", k)
	}

	for k, v := range common.RPBundle.Parameters {
		if _, ok := params[k]; !ok && v.Required && v.AppliesTo(action) {
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
	rpInput := r.Context().Value(models.BundleContext).(*models.BundleRP)
	log.Infof("Received POST Request: %s", rpInput.RequestPath)

	guid := rpInput.Properties.OperationId
	action := getAction(rpInput.RequestPath)
	status := fmt.Sprintf("Running%s", action)

	if rpInput.Properties.ProvisioningState != helpers.ProvisioningStateSucceeded {
		_ = render.Render(w, r, helpers.ErrorConflict(fmt.Sprintf("Cannot start action %s if provisioning state is not %s ", action, helpers.ProvisioningStateSucceeded)))
	}

	if len(rpInput.Properties.Status) == 0 {
		installationName := helpers.GetInstallationName(rpInput.Id)
		if exists, err := checkIfInstallationExists(installationName); err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to check for existing installation: %v", err)))
			return
		} else if !exists {
			// This should only happen if the resource is deleted outside of ARM
			// TODO clean-up state
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var args []string
		args = append(args, "invoke", installationName, "--action", action)

		if len(rpInput.Properties.Parameters) > 0 {
			if err := validateParameters(rpInput.Properties.Parameters, action); err != nil {
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
		guid = uuid.New().String()

		postData := jobs.PostJobData{
			RPInput:          rpInput,
			Args:             args,
			InstallationName: installationName,
			OperationId:      guid,
			Action:           action,
		}

		jobs.PostJobs <- &postData

		if err := azure.UpdateRPStatus(rpInput.SubscriptionId, rpInput.Id, status); err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to update state:%v", err)))
			return
		}

		if err := azure.PutAsyncOp(rpInput.SubscriptionId, guid, action, status, nil); err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to update async op %s :%v", guid, err)))
			return
		}
	} else {
		if !strings.EqualFold(status, rpInput.Properties.Status) {
			_ = render.Render(w, r, helpers.ErrorConflict(fmt.Sprintf("Cannot start action %s while status is %s", action, rpInput.Properties.Status)))
			return
		}
	}

	//TODO check that the action has the same parameters as the action that is running

	w.Header().Add("Retry-After", "60")
	w.Header().Add("Location", getLoctionHeader(rpInput, guid))
	w.WriteHeader(http.StatusAccepted)

}

func getAction(requestPath string) string {
	parts := strings.Split(requestPath, "/")
	return parts[len(parts)-1]
}

func deleteCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	rpInput := r.Context().Value(models.BundleContext).(*models.BundleRP)
	guid := rpInput.Properties.OperationId
	if rpInput.Properties.ProvisioningState != helpers.ProvisioningStateDeleting {

		installationName := helpers.GetInstallationName(rpInput.Id)
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
		if err := azure.PutAsyncOp(rpInput.SubscriptionId, guid, "delete", helpers.ProvisioningStateDeleting, nil); err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to update asyncop %s :%v", guid, err)))
			return
		}

	}

	w.Header().Add("Retry-After", "60")
	w.Header().Add("Location", getLoctionHeader(rpInput, guid))
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
	if state.Action == "delete" && (state.Status != helpers.ProvisioningStateDeleting && state.Status != helpers.AsyncOperationComplete) {
		_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Unexpected status for delete action op id %s :%v", rpInput.Name, state.Status)))
		return
	}
	if state.Action != "delete" && (!strings.EqualFold(state.Status, fmt.Sprintf("Running%s", state.Action)) && state.Status != helpers.AsyncOperationComplete) {
		if state.Output != nil {
			render.Status(r, http.StatusInternalServerError)
			output := make(map[string]interface{})
			output["Error"] = state.Output
			render.DefaultResponder(w, r, output)
		} else {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Unexpected status for action %s op id %s :%v", state.Action, rpInput.Name, state.Status)))
		}
		return
	}

	if state.Status == helpers.AsyncOperationComplete {
		if state.Action == "delete" {
			w.WriteHeader(http.StatusOK)
		} else {
			porterOutputs, err := helpers.GetBundleOutput(helpers.GetInstallationName(getResourceIdFromOperationsId(rpInput.Id)), []string{state.Action})
			if err != nil {
				_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to get outputs for action %s op id %s :%v", state.Action, rpInput.Name, err)))
				return
			}
			if state.Output != nil || len(porterOutputs) > 0 {
				var outputs = make(map[string]interface{})
				if state.Output != nil {
					outputs["output"] = state.Output
				}
				for _, v := range porterOutputs {
					outputs[v.Name] = v
				}
				render.Status(r, http.StatusOK)
				render.DefaultResponder(w, r, outputs)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}

		}
		return
	}

	// TODO deal with same action with different parameters

	w.Header().Add("Retry-After", "60")
	w.Header().Add("Location", fmt.Sprintf("https://management.azure.com%s&api-version=%s", rpInput.Id, helpers.APIVersion))
	w.WriteHeader(http.StatusAccepted)

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

func getLoctionHeader(rpInput *models.BundleRP, guid string) string {
	var location string
	host := rpInput.Properties.Host
	scheme := "https"
	if len(host) == 0 {
		port, exists := os.LookupEnv("LISTENER_PORT")
		if !exists {
			port = "8080"
		}
		host = fmt.Sprintf("localhost:%s", port)

	}
	if strings.HasPrefix(strings.ToLower(host), "localhost") {
		scheme = "http"
	}
	if len(guid) > 0 {
		location = fmt.Sprintf("%s://%s%s/operations/%s?api-version=%s", scheme, host, rpInput.Id, guid, helpers.APIVersion)
	} else {
		location = fmt.Sprintf("%s://%s%s?api-version=%s", scheme, host, rpInput.Id, helpers.APIVersion)
	}
	log.Debugf("Location Header:%s", location)
	return location
}

func getResourceIdFromOperationsId(requestPath string) string {
	parts := strings.Split(requestPath, "/")
	return strings.Join(parts[0:len(parts)-2], "/")
}
