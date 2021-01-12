package models

import (
	"context"
	"net/http"

	"github.com/go-chi/render"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/helpers"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/settings"
)

// ContextKey is the type used for the keys in the request context
type ContextKey string

const BundleContext ContextKey = "bundle"

// BundleCommandProperties defines the bundle and the properties to be used for the command
type BundleCommandProperties struct {
	Credentials                 map[string]interface{} `json:"credentials"`
	Parameters                  map[string]interface{} `json:"parameters"`
	ErrorResponse               *helpers.ErrorResponse `json:"-"`
	*settings.BundleInformation `json:"-"`
	ProvisioningState           string `json:"-"`
	OperationId                 string `json:"-"`
	Host                        string `json:"-"`
	CorrelationId               string `json:"-"`
	Error                       string `json:"error,omitempty"`
	Status                      string `json:"status,omitempty"`
}

type BundleCommandOutputs struct {
	Outputs map[string]interface{} `json:"properties,omitempty"`
}
type RPProperties struct {
	Id             string `json:"id"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	SubscriptionId string `json:"-"`
	RequestPath    string `json:"-"`
	RequestId      string `json:"-"`
}

type BundleRP struct {
	RPProperties
	Properties *BundleCommandProperties `json:"properties"`
}

type BundleRPOutput struct {
	*RPProperties
	*BundleCommandOutputs
}

func BundleCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := r.Context().Value(BundleContext).(*BundleRP)

		if r.ContentLength != 0 {
			if err := render.Bind(r, payload); err != nil {
				_ = render.Render(w, r, helpers.ErrorInvalidRequestFromError(err))
				return
			}
		} else {
			if err := payload.setRequestProperties(r); err != nil {
				_ = render.Render(w, r, helpers.ErrorInvalidRequestFromError(err))
				return
			}
		}

		ctx := context.WithValue(r.Context(), BundleContext, payload)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (bundleCommandProperties *BundleCommandProperties) Bind(r *http.Request) error {
	return nil
}

func (bundleCommandProperties *BundleCommandProperties) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func (payload *BundleRP) Bind(r *http.Request) error {
	return payload.setRequestProperties(r)
}

func (payload *BundleRP) setRequestProperties(r *http.Request) error {

	resource, resourceId, requestPath, err := helpers.GetResourceDetails(r)
	if err != nil {
		return err
	}

	payload.Properties.Host = r.Host
	payload.RequestPath = *requestPath
	payload.Id = *resourceId
	payload.Name = resource.ResourceName
	payload.SubscriptionId = resource.SubscriptionID
	payload.Type = resource.ResourceType

	return nil
}

func (payload *BundleRP) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func (payload *BundleRPOutput) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}
