package handlers

import (
	"fmt"
	"net/http"
	"os/exec"

	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/azure"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/models"
	log "github.com/sirupsen/logrus"
)

func NewCustomResourceHandler() chi.Router {

	r := chi.NewRouter()
	r.Use(render.SetContentType(render.ContentTypeJSON))
	r.Use(azure.Login)
	r.Use()
	r.Get("/*", getCustomResourceHandler)
	r.Put("/*", putCustomResourceHandler)
	r.Post("/*", postCustomResourceHandler)
	r.Delete("/*", deleteCustomResourceHandler)
	return r
}

func getCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	requestPath := r.Header.Get("x-ms-customproviders-requestpath")
	log.Infof("Received Request: %s", requestPath)
	render.DefaultResponder(w, r, fmt.Sprintf("Get Hello World!! from %s", r.URL.String()))
}

func putCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	bundleCommandProperties := r.Context().Value(models.BundleContext).(*models.BundleCommandProperties)
	log.Infof("Received Request: %s", bundleCommandProperties.RequestPath)
	render.Render(w, r, bundleCommandProperties)
}

func postCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Received Request: %s", r.RequestURI)
	render.DefaultResponder(w, r, fmt.Sprintf("Post Hello World!! from %s", r.RequestURI))
}

func deleteCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Received Request: %s", r.RequestURI)
	render.DefaultResponder(w, r, fmt.Sprintf("Delete Hello World!! from %s", r.RequestURI))
}

func executePorterCommand(args []string) error {
	args = append(args, "-d")
	args = append(args, "azure")
	args = append(args, "-o")
	args = append(args, "json")
	cmd := exec.Command("porter", args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Porter start failed: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("Porter command failed: %v", err)
	}
	return nil
}
