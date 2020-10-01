package models

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"get.porter.sh/porter/pkg/porter"
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
	*porter.BundlePullOptions `json:"-"`
	RequestPath               string `json:"-"`
}

type BundleCommandOutputs struct {
	Outputs map[string]interface{} `json:"outputs,omitempty"`
}
type RPProperties struct {
	Id           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Installation string `json:"Installation"`
}
type BundleRP struct {
	RPProperties
	Properties *BundleCommandProperties `json:"properties"`
}

type BundleRPOutput struct {
	*RPProperties
	Properties *BundleCommandOutputs `json:"properties"`
}

func BundleCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		payload := &BundleRP{
			Properties: &BundleCommandProperties{},
		}

		if err := render.Bind(r, payload); err != nil {
			_ = render.Render(w, r, helpers.ErrorInvalidRequestFromError(err))
			return
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
	requestPath := r.Header.Get("x-ms-customproviders-requestpath")
	if len(requestPath) == 0 {
		return errors.New("x-ms-customproviders-requestpath missing from request")
	}

	// TODO update to use SDK
	payload.Id = requestPath
	resourceIDParts := strings.Split(requestPath, "/")
	payload.Name = resourceIDParts[len(resourceIDParts)-1]
	payload.Type = fmt.Sprintf("%s/%s", resourceIDParts[len(resourceIDParts)-3], resourceIDParts[len(resourceIDParts)-2])
	payload.Properties.RequestPath = requestPath

	return nil
}

func (payload *BundleRP) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func (payload *BundleRPOutput) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}
