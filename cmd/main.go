package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg"
	az "github.com/simongdavies/cnab-custom-resource-handler/pkg/azure"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/handlers"

	"github.com/simongdavies/cnab-custom-resource-handler/pkg/jobs"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/settings"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var debug bool
var rootCmd = &cobra.Command{
	Use:   "cnabcustomrphandler",
	Short: "Launches a web server that provides ARM RPC compliant CRUD endpoints for a CNAB Bundle",
	Long:  `Launches a web server that provides ARM RPC compliant CRUD endpoints for a CNAB Bundle which can be used as an ARM Custom resource provider implementation for CNAB`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		log.SetReportCaller(true)
		if debug {
			log.SetLevel(log.DebugLevel)
		}
		log.Debugf("Commit:%s Version:%s", pkg.Commit, pkg.Version)
		port, exists := os.LookupEnv("LISTENER_PORT")
		if !exists {
			port = "8080"
		}
		if err := settings.Load(); err != nil {
			log.Errorf("Error loading settings %v", err)
			return err
		}

		if err := az.SetAzureStorageInfo(); err != nil {
			log.Errorf("Error setting storage connection settings %v", err)
			return err
		}

		jobs.Start()
		log.Debug("Creating Router")
		router := chi.NewRouter()
		router.Use(az.LogRequestBody)
		router.Use(az.LogResponseBody)
		router.Use(az.RequestId)
		router.Use(middleware.RealIP)
		router.Use(middleware.Logger)
		router.Use(az.Login)
		router.Use(middleware.Timeout(10 * time.Minute))
		router.Use(middleware.Recoverer)
		log.Debug("Creating Handler")
		router.Handle("/*", handlers.NewCustomResourceHandler())
		log.Infof("Starting to listen on port  %s", port)
		err := http.ListenAndServe(fmt.Sprintf(":%s", port), router)
		if err != nil {
			log.Errorf("Error running HTTP Server %v", err)
			return err
		}
		jobs.Stop()
		return nil
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "specifies if debug output should be produced")
}
