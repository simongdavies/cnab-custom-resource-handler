package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	az "github.com/Azure/go-autorest/autorest/azure"
	"github.com/cnabio/cnab-go/bundle"
	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/azure"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/helpers"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/jobs"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/models"
	log "github.com/sirupsen/logrus"
)

func NewCustomResourceHandler() chi.Router {
	r := chi.NewRouter()
	r.Use(render.SetContentType(render.ContentTypeJSON))
	r.Use(azure.ValidateRPType)
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
	log.Infof("GET Request URI: %s", r.URL.String())

	if azure.IsOperationsRequest(rpInput.Id) {
		getOperationHandler(w, r)
		return
	}

	// TODO change to use Custom RP Type name
	if rpInput.Name == "installs" {
		listCustomResourceHandler(w, r, rpInput.SubscriptionId)
		return
	}

	installationName := helpers.GetInstallationName(rpInput.Properties.TrimmedBundleTag, rpInput.Id)

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

	rpOutput, err := getRPOutput(rpInput.Properties.BundleInformation.RPBundle, installationName, rpInput, rpInput.Properties.ProvisioningState)
	if err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed getRPOutput: %v", err)))
		return
	}
	render.DefaultResponder(w, r, rpOutput)
}

func listCustomResourceHandler(w http.ResponseWriter, r *http.Request, subscriptionId string) {
	rpInput := r.Context().Value(models.BundleContext).(*models.BundleRP)
	log.Infof("Received LIST Request: %s", rpInput.RequestPath)
	log.Infof("LIST Request URI: %s", r.URL.String())
	// TODO handle paging
	res, err := azure.ListRPState(subscriptionId, rpInput.Properties.BundleInformation.ResourceProvider, rpInput.Properties.BundleInformation.ResourceType)
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
	log.Infof("PUT Request URI: %s", r.URL.String())
	installationName := helpers.GetInstallationName(rpInput.Properties.TrimmedBundleTag, rpInput.Id)

	action := "install"
	provisioningState := "Created"
	if exists, err := checkIfInstallationExists(installationName); err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to check for existing installation: %v", err)))
		return
	} else if exists {
		action = "upgrade"
		provisioningState = "Accepted"
	}

	var args []string
	args = append(args, action, installationName, "--reference", rpInput.Properties.BundlePullOptions.Tag)

	if len(rpInput.Properties.Parameters) > 0 {
		if err := validateParameters(rpInput.Properties.BundleInformation.RPBundle, rpInput.Properties.Parameters, action); err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to validate parameters:%v", err)))
			return
		}
	}

	if len(rpInput.Properties.Credentials) > 0 {
		if err := validateCredentials(rpInput.Properties.BundleInformation.RPBundle, rpInput.Properties.Credentials); err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to validate credentials:%v", err)))
			return
		}
	}

	jobData := jobs.PutJobData{
		RPInput:          rpInput,
		Args:             args,
		InstallationName: installationName,
	}

	rpInput.Properties.ProvisioningState = provisioningState
	if err := azure.PutRPState(rpInput.SubscriptionId, rpInput.Id, rpInput.Properties); err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to update state:%v", err)))
		return
	}

	jobs.PutJobs <- &jobData

	rpOutput, err := getRPOutput(rpInput.Properties.BundleInformation.RPBundle, installationName, rpInput, provisioningState)
	if err != nil {
		_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to get RP output:%v", err)))
		return
	}

	status := http.StatusCreated
	if action == "upgrade" {
		status = http.StatusOK
	}

	// Using provisioning state to show that operation is async, no location header reqruied  (see https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/Addendum.md#creatingupdating-using-put)
	render.Status(r, status)
	render.DefaultResponder(w, r, rpOutput)

}

func getRPOutput(rpBundle *bundle.Bundle, installationName string, rpInput *models.BundleRP, provisioningState string) (*models.BundleRPOutput, error) {

	var cmdOutput []helpers.PorterOutput
	var err error

	// TODO: get state from table storage (state sync needed)
	if provisioningState == helpers.ProvisioningStateSucceeded {
		cmdOutput, err = helpers.GetBundleOutput(rpBundle, installationName, []string{"install", "upgrade"})
		if err != nil {
			return nil, err
		}
	}

	output := make(map[string]interface{})

	output["ProvisioningState"] = provisioningState
	output["Installation"] = installationName
	for _, v := range cmdOutput {
		log.Debugf("Installation Name:%s Output:%s", installationName, v.Name)
		if IsSenstive, _ := rpBundle.IsOutputSensitive(v.Name); !IsSenstive {
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

func validateCredentials(rpBundle *bundle.Bundle, creds map[string]interface{}) error {

	for k := range rpBundle.Credentials {
		log.Debugf("Credential Name:%s", k)
	}

	for k, v := range rpBundle.Credentials {
		if _, ok := creds[k]; !ok && v.Required {
			log.Debugf("Credential %s is required", k)
			return fmt.Errorf("Credential %s is required", k)
		}
	}

	for k := range creds {
		if _, ok := rpBundle.Credentials[k]; !ok {
			log.Debugf("Credential %s is not specified in bundle", k)
			return fmt.Errorf("Credential %s is not specified in bundle", k)
		}
	}
	return nil
}

func validateParameters(rpBundle *bundle.Bundle, params map[string]interface{}, action string) error {

	for k := range rpBundle.Parameters {
		log.Debugf("Processing parameter name:%s", k)
	}

	for k, v := range rpBundle.Parameters {
		if _, ok := params[k]; !ok && v.Required && v.AppliesTo(action) {
			log.Debugf("Parameter Name:%s Value is required", k)
			return fmt.Errorf("Parameter %s is required", k)
		}
	}

	for k := range params {
		if _, ok := rpBundle.Parameters[k]; !ok {
			log.Debugf("Parameter Name:%s Value not specified in bundle", k)
			return fmt.Errorf("Parameter %s is not specified in bundle", k)
		}
	}
	return nil
}

func postCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	rpInput := r.Context().Value(models.BundleContext).(*models.BundleRP)
	log.Infof("Received POST Request: %s", rpInput.RequestPath)
	log.Infof("POST Request URI: %s", r.URL.String())

	guid := rpInput.Properties.OperationId
	action := getAction(rpInput.RequestPath)
	status := fmt.Sprintf("Running%s", action)

	if rpInput.Properties.ProvisioningState != helpers.ProvisioningStateSucceeded {
		_ = render.Render(w, r, helpers.ErrorConflict(fmt.Sprintf("Cannot start action %s if provisioning state is not %s ", action, helpers.ProvisioningStateSucceeded)))
	}

	if len(rpInput.Properties.Status) == 0 {
		installationName := helpers.GetInstallationName(rpInput.Properties.TrimmedBundleTag, rpInput.Id)
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
			if err := validateParameters(rpInput.Properties.BundleInformation.RPBundle, rpInput.Properties.Parameters, action); err != nil {
				_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to validate parameters:%v", err)))
				return
			}
		}

		if len(rpInput.Properties.Credentials) > 0 {
			if err := validateCredentials(rpInput.Properties.BundleInformation.RPBundle, rpInput.Properties.Credentials); err != nil {
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
	w.Header().Add("Location", getLocationHeader(rpInput, guid))
	w.WriteHeader(http.StatusAccepted)

}

func getAction(requestPath string) string {
	parts := strings.Split(requestPath, "/")
	return parts[len(parts)-1]
}

func deleteCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	rpInput := r.Context().Value(models.BundleContext).(*models.BundleRP)
	log.Infof("Received DELETE Request: %s", rpInput.RequestPath)
	log.Infof("DELETE Request URI: %s", r.URL.String())
	guid := rpInput.Properties.OperationId
	if rpInput.Properties.ProvisioningState != helpers.ProvisioningStateDeleting {

		installationName := helpers.GetInstallationName(rpInput.Properties.TrimmedBundleTag, rpInput.Id)
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
			BundleInfo:       rpInput.Properties.BundleInformation,
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
	w.Header().Add("Location", getLocationHeader(rpInput, guid))
	w.WriteHeader(http.StatusAccepted)

}

func getOperationHandler(w http.ResponseWriter, r *http.Request) {

	// TODO validate the operation Id

	rpInput := r.Context().Value(models.BundleContext).(*models.BundleRP)
	log.Infof("Received GET Operation Request: %s", rpInput.RequestPath)
	log.Infof("Operation Request URI: %s", r.URL.String())

	operation := models.Operation{
		Id:   rpInput.Id,
		Name: rpInput.Name,
	}

	state, err := azure.GetAsyncOp(rpInput.SubscriptionId, rpInput.Name)
	if err != nil {
		writeOperation(w, r, &operation, helpers.AsyncOperationUnknown, "InternalServerError", fmt.Sprintf("Failed to get async op %s :%v", rpInput.Name, err), http.StatusInternalServerError)
		return
	}
	if state.Action == "delete" && (state.Status != helpers.ProvisioningStateDeleting && state.Status != helpers.AsyncOperationComplete && state.Status != helpers.AsyncOperationFailed) {
		writeOperation(w, r, &operation, helpers.AsyncOperationUnknown, "InternalServerError", fmt.Sprintf("Unexpected status for delete action op id %s :%v", rpInput.Name, state.Status), http.StatusInternalServerError)
		return
	}
	if state.Action != "delete" && (!strings.EqualFold(state.Status, fmt.Sprintf("Running%s", state.Action)) && state.Status != helpers.AsyncOperationComplete && state.Status != helpers.AsyncOperationFailed) {
		if len(state.Output) > 0 {
			writeOperation(w, r, &operation, helpers.AsyncOperationFailed, "InternalServerError", state.Output, http.StatusInternalServerError)
		} else {
			writeOperation(w, r, &operation, helpers.AsyncOperationUnknown, "InternalServerError", fmt.Sprintf("Unexpected status for action %s op id %s :%v", state.Action, rpInput.Name, state.Status), http.StatusInternalServerError)
		}
		return
	}

	if state.Status == helpers.AsyncOperationComplete || state.Status == helpers.StatusFailed {
		operation.Status = state.Status
		if state.Action != "delete" {
			porterOutputs, err := helpers.GetBundleOutput(rpInput.Properties.BundleInformation.RPBundle, helpers.GetInstallationName(rpInput.Properties.TrimmedBundleTag, getResourceIdFromOperationsId(rpInput.Id)), []string{state.Action})
			if err != nil {
				writeOperation(w, r, &operation, state.Status, "InternalServerError", fmt.Sprintf("Failed to get outputs for action %s op id %s :%v", state.Action, rpInput.Name, err), http.StatusInternalServerError)
				return
			}
			if len(state.Output) > 0 || len(porterOutputs) > 0 {
				operation.Properties = make(map[string]interface{})
				if len(state.Output) > 0 {
					operation.Properties["output"] = state.Output
				}
				for _, v := range porterOutputs {
					operation.Properties[v.Name] = &v
				}
				render.Status(r, http.StatusOK)
			}
		}
		render.Status(r, http.StatusOK)
		render.DefaultResponder(w, r, operation)
		return
	}

	// TODO deal with same action with different parameters

	operation.Status = state.Status
	w.Header().Add("Retry-After", "60")
	w.Header().Add("Location", getLocationHeader(rpInput, ""))
	render.Status(r, http.StatusAccepted)
	render.DefaultResponder(w, r, &operation)

}

func writeOperation(w http.ResponseWriter, r *http.Request, operation *models.Operation, status string, code string, message string, statuscode int) {
	operation.Status = status
	operation.Error = &models.OperationError{
		Code:    code,
		Message: message,
	}
	render.Status(r, statuscode)
	render.DefaultResponder(w, r, &operation)
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

func getLocationHeader(rpInput *models.BundleRP, guid string) string {
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
