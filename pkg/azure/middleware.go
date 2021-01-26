package azure

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/storage"
	az "github.com/Azure/go-autorest/autorest/azure"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/helpers"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/models"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/settings"
	log "github.com/sirupsen/logrus"
)

type AzureLoginContextKey string

const AzureLoginContext AzureLoginContextKey = "AzureLoginContext"

type ResponseLogger struct {
	w http.ResponseWriter
}

func NewResponseLogger(writer http.ResponseWriter) *ResponseLogger {
	return &ResponseLogger{
		w: writer,
	}
}

func (r *ResponseLogger) Write(b []byte) (int, error) {
	log.Debugf("Response Body:%s", string(b))
	return r.w.Write(b)
}

func (r *ResponseLogger) Header() http.Header {
	return r.w.Header()
}

func (r *ResponseLogger) WriteHeader(statusCode int) {
	r.w.WriteHeader(statusCode)
}

// HTTP middleware setting original request URL on context
func Login(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var loginInfo LoginInfo
		var err error
		if loginInfo, err = LoginToAzure(); err != nil {
			log.Infof("Failed to Login: %v", err)
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to Login to Azure error: %v for request URI %s", err, r.RequestURI)))
		}
		log.Debugf("Logged in to Azure for request URI %s", r.RequestURI)
		ctx := context.WithValue(r.Context(), AzureLoginContext, loginInfo)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LogResponseBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writer := w
		if settings.LogResponseBody {
			writer = NewResponseLogger(w)
		}
		next.ServeHTTP(writer, r)

	})
}

func LogRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if settings.LogRequestBody {
			if body, err := ioutil.ReadAll(r.Body); err != nil {
				log.Debug("Error Logging Request Body:%w", err)
			} else {
				r.Body = ioutil.NopCloser(bytes.NewBuffer(body))
				log.Debugf("Request Body:%s", string(body))
			}
		}
		next.ServeHTTP(w, r)
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

		payload := &models.BundleRP{
			Properties: &models.BundleCommandProperties{},
		}
		ctx := context.WithValue(r.Context(), models.BundleContext, payload)
		requestPath := r.URL.Path
		resource, err := az.ParseResourceID(requestPath)
		if err != nil {
			log.Infof("Failed to parse request path: %s Error: %v", requestPath, err)
			_ = render.Render(w, r, helpers.ErrorInternalServerError(fmt.Sprintf("Failed to parse request path: %s Error: %v", requestPath, err)))
			return
		}

		var bundleInfo *settings.BundleInformation
		var ok bool
		if settings.IsRPaaS {
			rpName := settings.GetRPName(resource.Provider, resource.ResourceType)
			bundleInfo, ok = settings.RPToProvider[rpName]
			if !ok {
				log.Infof("no mapping found for request: %s Provider:%s", requestPath, rpName)
				_ = render.Render(w, r, helpers.ErrorInternalServerError(fmt.Sprintf("no mapping found for request: %s Provider:%s", requestPath, rpName)))
				return
			}
			log.Debugf("Using Bundle %s to process request", bundleInfo.BundlePullOptions.Tag)
		} else {
			resource.ResourceType = strings.Split(requestPath, "/")[8]
			rpName := settings.GetRPName(resource.Provider, resource.ResourceType)
			bundleInfo, ok = settings.RPToProvider[rpName]
			if !ok || !strings.EqualFold(resource.Provider, bundleInfo.ResourceProvider) || !strings.EqualFold(resource.ResourceType, bundleInfo.ResourceType) {
				log.Infof("request: %s not for registered Resource Provider %s Resource Type:%s", requestPath, bundleInfo.ResourceProvider, bundleInfo.ResourceType)
				_ = render.Render(w, r, helpers.ErrorInternalServerError(fmt.Sprintf("request: %s not for registered Resource Provider %s Resource Type:%s", requestPath, bundleInfo.ResourceProvider, bundleInfo.ResourceType)))
				return
			}
		}

		payload.Properties.BundleInformation = bundleInfo
		if strings.Contains(requestPath, "!") {
			log.Infof("request: %s contains !", requestPath)
			_ = render.Render(w, r, helpers.ErrorInternalServerError(fmt.Sprintf("resource name: %s is not valid ! character is not allowed", requestPath)))
			return
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoadState(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		payload := r.Context().Value(models.BundleContext).(*models.BundleRP)

		resource, requestId, requestPath, err := helpers.GetResourceDetails(r)
		if err != nil {
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(err))
			return
		}

		// List request

		if r.Method == "GET" && IsListRequest(*requestPath) {
			next.ServeHTTP(w, r)
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
			installationName := helpers.GetInstallationName(payload.Properties.TrimmedBundleTag, *requestId)
			outputs, err := helpers.GetBundleOutput(payload.Properties.BundleInformation.RPBundle, installationName, []string{"install", "upgrade"})
			if err != nil {
				_ = render.Render(w, r, helpers.ErrorInternalServerError(fmt.Sprintf("Failed to get bundle outputs is: %v", err)))
				return
			}
			for _, v := range outputs {
				log.Debugf("Installation Name:%s Output:%s", installationName, v.Name)
				if IsSenstive, _ := payload.Properties.BundleInformation.RPBundle.IsOutputSensitive(v.Name); !IsSenstive {
					payload.Properties.Parameters[v.Name] = strings.TrimSuffix(v.Value, "\\n")
				}
			}
		}

		next.ServeHTTP(w, r)
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

func IsListRequest(requestPath string) bool {
	parts := strings.Split(requestPath, "/")
	return len(parts)%2 == 0
}
