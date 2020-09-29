package azure

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/render"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/helpers"
	log "github.com/sirupsen/logrus"
)

type AzureLoginContextKey string

const AzureLoginContext AzureLoginContextKey = "AzureLoginContext"

// HTTP middleware setting original request URL on context
func Login(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var loginInfo LoginInfo
		var err error
		if loginInfo, err = LoginToAzure(); err != nil {
			log.Infof("Failed to Login: %v", err)
			_ = render.Render(w, r, helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to Loginto Azure error: %v", err)))
		}
		log.Debug("Logged in to Azure")
		ctx := context.WithValue(r.Context(), AzureLoginContext, loginInfo)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
