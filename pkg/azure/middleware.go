package azure

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/common"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/helpers"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/models"
	log "github.com/sirupsen/logrus"
)

type AzureLoginContextKey string

const AzureLoginContext AzureLoginContextKey = "AzureLoginContext"

var RPType string
var IsRPaaS bool

// HTTP middleware setting original request URL on context
func Login(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var loginInfo LoginInfo
		var err error
		if loginInfo, err = LoginToAzure(); err != nil {
			log.Infof("Failed to Login: %v", err)
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to Loginto Azure error: %v for request URI %s", err, r.RequestURI)))
		}
		log.Debugf("Logged in to Azure for request URI %s", r.RequestURI)
		ctx := context.WithValue(r.Context(), AzureLoginContext, loginInfo)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequestId(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		requestId := r.Header.Get("X-Ms-Correlation-Request-Id")
		if len(requestId) == 0 {
			requestId = uuid.New().String()
		}
		r.Header.Set(middleware.RequestIDHeader, requestId)
		ctx = context.WithValue(ctx, middleware.RequestIDKey, requestId)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ValidateRPType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		requestPath := r.URL.Path

		// TODO Handle multiple providers/types for RPaaS

		if IsRPaaS {
			if !strings.Contains(strings.ToLower(requestPath), strings.ToLower(RPType)) {
				log.Infof("request: %s not for registered Provider:%s", requestPath, RPType)
				_ = render.Render(w, r, helpers.ErrorInternalServerError(fmt.Sprintf("request: %s not for registered Provider:%s", requestPath, RPType)))
				return
			}
		} else {
			if !strings.HasPrefix(strings.ToLower(requestPath), strings.ToLower(RPType)) {
				log.Infof("request: %s not for registered RP Type:%s", requestPath, RPType)
				_ = render.Render(w, r, helpers.ErrorInternalServerError(fmt.Sprintf("request: %s not for registered RP Type:%s", requestPath, RPType)))
				return
			}
		}

		if strings.Contains(requestPath, "!") {
			log.Infof("request: %s contains !", requestPath)
			_ = render.Render(w, r, helpers.ErrorInternalServerError(fmt.Sprintf("resource name: %s is not valid ! character is not allowed", requestPath)))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func LoadState(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		payload := &models.BundleRP{
			Properties: &models.BundleCommandProperties{},
		}
		ctx := context.WithValue(r.Context(), models.BundleContext, payload)

		resource, requestId, requestPath, err := helpers.GetResourceDetails(r)
		if err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(err))
			return
		}

		// List request
		// TODO get the resource name from a setting/metadata

		if r.Method == "GET" && resource.ResourceName == "installs" {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		properties, err := GetRPState(resource.SubscriptionID, *requestId)

		if err != nil {
			storageError, ok := err.(storage.AzureStorageServiceError)
			if !ok || ok && storageError.StatusCode != 404 {
				_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to get RPState: %v", err)))
				return
			}
			if ok && storageError.StatusCode == 404 && r.Method != "PUT" && //Not found is valid for 1st Put (Create)
				!(r.Method == "GET" && IsOperationsRequest(*requestPath)) { // Get on operations will produce not found
				_ = render.Render(w, r, helpers.ErrorNotFound())
				return
			}

		}

		switch r.Method {
		case "PUT":
			{
				// properties will be nil on first PUT of a resource
				if properties != nil && !IsTerminalProvisioningState(properties.ProvisioningState) {
					_ = render.Render(w, r, helpers.ErrorConflict(fmt.Sprintf("Resource Provisioning State is: %s", properties.ProvisioningState)))
					return
				}
			}
		case "POST":
			{
				// properties will be nil on first PUT of a resource
				if !IsTerminalProvisioningState(properties.ProvisioningState) {
					_ = render.Render(w, r, helpers.ErrorConflict(fmt.Sprintf("Resource Provisioning State is: %s", properties.ProvisioningState)))
					return
				}
			}
		case "DELETE":
			{
				// Deleting is fine if the resource is already being deleted
				if !IsTerminalProvisioningState(properties.ProvisioningState) && properties.ProvisioningState != helpers.ProvisioningStateDeleting {
					_ = render.Render(w, r, helpers.ErrorConflict(fmt.Sprintf("Resource Provisioning State is: %s", properties.ProvisioningState)))
					return
				}
			}
		}

		if properties != nil {
			payload.Properties.ProvisioningState = properties.ProvisioningState
			payload.Properties.Credentials = properties.Credentials
			payload.Properties.Parameters = properties.Parameters
			payload.Properties.ErrorResponse = properties.ErrorResponse
			payload.Properties.OperationId = properties.OperationId
			installationName := helpers.GetInstallationName(*requestId)
			outputs, err := helpers.GetBundleOutput(installationName, []string{"install", "upgrade"})
			if err != nil {
				_ = render.Render(w, r, helpers.ErrorInternalServerError(fmt.Sprintf("Failed to get bundle outputs is: %v", err)))
				return
			}
			for _, v := range outputs {
				log.Debugf("Installation Name:%s Output:%s", installationName, v.Name)
				if IsSenstive, _ := common.RPBundle.IsOutputSensitive(v.Name); !IsSenstive {
					payload.Properties.Parameters[v.Name] = strings.TrimSuffix(v.Value, "\\n")
				}
			}
		}

		payload.Properties.BundlePullOptions = common.BundlePullOptions

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func IsTerminalProvisioningState(provisioningState string) bool {
	//TODO handle POST Action executing
	return provisioningState == helpers.ProvisioningStateFailed || provisioningState == helpers.ProvisioningStateSucceeded
}

func IsOperationsRequest(requestPath string) bool {
	parts := strings.Split(requestPath, "/")
	// Operations Request
	return parts[len(parts)-2] == "operations"
}
