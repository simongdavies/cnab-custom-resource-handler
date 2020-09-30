package models

import (
	"context"
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

type CNABRP struct {
	Id         string                   `json:"id"`
	Name       string                   `json:"name"`
	Type       string                   `json:"type"`
	Properties *BundleCommandProperties `json:"properties"`
}

func BundleCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		bundleCommandProperties := &BundleCommandProperties{}
		if err := render.Bind(r, bundleCommandProperties); err != nil {
			render.Render(w, r, helpers.ErrorInvalidRequestFromError(err))
			return
		}
		requestPath := r.Header.Get("x-ms-customproviders-requestpath")
		if len(requestPath) == 0 {
			_ = render.Render(w, r, helpers.ErrorInvalidRequest("x-ms-customproviders-requestpath missing from request"))
		}

		// TODO update to use library

		resourceIDParts := strings.Split(requestPath, "/")
		resourceName := resourceIDParts[len(resourceIDParts)-1]
		resourceType := fmt.Sprintf("%s/%s", resourceIDParts[len(resourceIDParts)-3], resourceIDParts[len(resourceIDParts)-2])

		payload := CNABRP{
			Id:         requestPath,
			Name:       resourceName,
			Type:       resourceType,
			Properties: bundleCommandProperties,
		}
		bundleCommandProperties.RequestPath = requestPath
		ctx := context.WithValue(r.Context(), BundleContext, &payload)
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

func (payload *CNABRP) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}
