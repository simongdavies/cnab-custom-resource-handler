package models

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"get.porter.sh/porter/pkg/porter"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/go-chi/render"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/common"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/helpers"
)

// ContextKey is the type used for the keys in the request context
type ContextKey string

const BundleContext ContextKey = "bundle"

// BundleCommandProperties defines the bundle and the properties to be used for the command
type BundleCommandProperties struct {
	Credentials               map[string]interface{} `json:"credentials"`
	Parameters                map[string]interface{} `json:"parameters"`
	ErrorResponse             *helpers.ErrorResponse `json:"-"`
	*porter.BundlePullOptions `json:"-"`
	ProvisioningState         string `json:"-"`
	OperationId               string `json:"-"`
	Error                     string `json:"error,omitempty"`
	Status                    string `json:"status,omitempty"`
}

type BundleCommandOutputs struct {
	Outputs map[string]interface{} `json:"properties,omitempty"`
}
type RPProperties struct {
	Id             string `json:"id"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	SubscriptionId string `json:"-"`
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
			if err := payload.setResource(r); err != nil {
				_ = render.Render(w, r, helpers.ErrorInvalidRequestFromError(err))
				return
			}
		}

		ctx := context.WithValue(r.Context(), BundleContext, payload)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (bundleCommandProperties *BundleCommandProperties) Bind(r *http.Request) error {
	bundleCommandProperties.BundlePullOptions = common.BundlePullOptions
	return nil
}

func (bundleCommandProperties *BundleCommandProperties) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func (payload *BundleRP) Bind(r *http.Request) error {
	return payload.setResource(r)
}

func (payload *BundleRP) setResource(r *http.Request) error {
	requestPath := r.Header.Get("x-ms-customproviders-requestpath")
	resource, err := azure.ParseResourceID(requestPath)
	if err != nil {
		return fmt.Errorf("Failed to parse x-ms-customproviders-requestpath: %v", err)
	}
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = fmt.Sprintf("%s%s", "/", requestPath)
	}
	requestParts := strings.Split(requestPath, "/")

	payload.Id = requestPath
	payload.Name = resource.ResourceName
	payload.SubscriptionId = resource.SubscriptionID
	payload.Type = strings.Join(requestParts[6:len(requestParts)-1], "/")

	return nil
}

func (payload *BundleRP) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func (payload *BundleRPOutput) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}
