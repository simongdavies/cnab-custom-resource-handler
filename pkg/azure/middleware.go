package azure

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/render"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/helpers"
	log "github.com/sirupsen/logrus"
)

type AzureLoginContextKey string

const AzureLoginContext AzureLoginContextKey = "AzureLoginContext"

var RPType string

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

func ValidateRPType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath := r.Header.Get("x-ms-customproviders-requestpath")
		if len(requestPath) == 0 {
			log.Info("Header x-ms-customproviders-requestpath isssing from request")
			_ = render.Render(w, r, helpers.ErrorInternalServerError("Header x-ms-customproviders-requestpath missing from request"))
			return
		}
		if !strings.HasPrefix(strings.ToLower(strings.TrimPrefix(requestPath, "/")), strings.ToLower(strings.TrimPrefix(RPType, "/"))) {
			log.Infof("request: %s not for registered RP Type:%s", requestPath, RPType)
			_ = render.Render(w, r, helpers.ErrorInternalServerError(fmt.Sprintf("request: %s not for registered RP Type:%s", requestPath, RPType)))
			return
		}
		next.ServeHTTP(w, r)
	})
}
