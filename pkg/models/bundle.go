package models

import (
	"context"
	"net/http"

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
	Credentials map[string]string `json:"credentials"`
	Parameters  map[string]string `json:"parameters"`
	*porter.BundlePullOptions
	RequestPath string
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
			_ = render.Render(w, r, helpers.ErrorInvalidRequest("x-ms-customproviders-requestpathmissing from request"))
		}
		bundleCommandProperties.RequestPath = requestPath
		ctx := context.WithValue(r.Context(), BundleContext, bundleCommandProperties)
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
