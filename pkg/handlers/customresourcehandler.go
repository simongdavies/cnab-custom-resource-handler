package handlers

import (
	"net/http"

	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	log "github.com/sirupsen/logrus"
)

func NewCustomResourceHandler() chi.Router {

	r := chi.NewRouter()
	r.Use(render.SetContentType(render.ContentTypeJSON))
	r.Get("/*", getCustomResourceHandler)
	r.Put("/*", putCustomResourceHandler)
	r.Post("/*", postCustomResourceHandler)
	r.Delete("/*", deleteCustomResourceHandler)
	return r
}

func getCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Received Request: %s", r.RequestURI)
	_, _ = w.Write([]byte("Get Hello World!!"))
}

func putCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Received Request: %s", r.RequestURI)
	_, _ = w.Write([]byte("Put Hello World!!"))
}

func postCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Received Request: %s", r.RequestURI)
	_, _ = w.Write([]byte("Post Hello World!!"))
}

func deleteCustomResourceHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Received Request: %s", r.RequestURI)
	_, _ = w.Write([]byte("Delete Hello World!!"))
}
