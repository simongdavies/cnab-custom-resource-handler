package jobs

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/simongdavies/cnab-custom-resource-handler/pkg/azure"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/common"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/helpers"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/models"
	log "github.com/sirupsen/logrus"
)

type PutJobData struct {
	RPInput          *models.BundleRP
	Args             []string
	InstallationName string
}

var PutJobs chan *PutJobData = make(chan *PutJobData, 20)

func startPutJob() {
	for i := 0; i < MaxJobs; i++ {
		go func(putJobs chan *PutJobData, i int) {
			log.Debugf("Starting Put Job %d", i)
			for jobData := range putJobs {
				log.Debugf("Starting Put Resource Job for %s", jobData.RPInput.Id)
				putJob(jobData)
				log.Debugf("Finished Put Resource Job for %s", jobData.RPInput.Id)
			}
			log.Debugf("Stopped Put Job %d", i)
		}(PutJobs, i)
	}
}

func putJob(jobData *PutJobData) {

	log.Debugf("Started processing PUT request for %s", jobData.RPInput.Id)

	//TODO Implement Timeouts
	jobData.RPInput.Properties.ProvisioningState = helpers.ProvisioningStateFailed
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		responseError := helpers.ErrorInternalServerErrorFromError(fmt.Errorf("error creating temp dir: %v", err))
		if err := azure.SetFailedProvisioningState(jobData.RPInput.SubscriptionId, jobData.RPInput.Id, responseError); err != nil {
			log.Debugf("Failed to Merge RP State for response error %v: %v", responseError, err)
		}
		return
	}
	defer os.RemoveAll(dir)

	if len(jobData.RPInput.Properties.Parameters) > 0 {
		paramFile, err := common.WriteParametersFile(jobData.RPInput.Properties.BundleInformation.RPBundle, jobData.RPInput.Properties.Parameters, dir)
		if err != nil {
			responseError := helpers.ErrorInternalServerErrorFromError(err)
			if err := azure.SetFailedProvisioningState(jobData.RPInput.SubscriptionId, jobData.RPInput.Id, responseError); err != nil {
				log.Debugf("Failed to Merge RP State for response error %v: %v", responseError, err)
			}
			return
		}
		jobData.Args = append(jobData.Args, "-p", paramFile.Name())
		defer os.Remove(paramFile.Name())
	}

	if len(jobData.RPInput.Properties.Credentials) > 0 {
		credFile, err := common.WriteCredentialsFile(jobData.RPInput.Properties.BundleInformation.RPBundle, jobData.RPInput.Properties.Credentials, dir)
		if err != nil {
			responseError := helpers.ErrorInternalServerErrorFromError(err)
			if err := azure.SetFailedProvisioningState(jobData.RPInput.SubscriptionId, jobData.RPInput.Id, responseError); err != nil {
				log.Debugf("Failed to Merge RP State for response error %v: %v", responseError, err)
			}
			return
		}
		jobData.Args = append(jobData.Args, "-c", credFile.Name())
		defer os.Remove(credFile.Name())
	}

	if out, err := helpers.ExecutePorterCommand(jobData.Args); err != nil {
		responseError := helpers.ErrorInternalServerError(string(out))
		if err := azure.SetFailedProvisioningState(jobData.RPInput.SubscriptionId, jobData.RPInput.Id, responseError); err != nil {
			log.Debugf("Failed to Merge RP State for response error %v: %v", responseError, err)
		}
		return
	}
	jobData.RPInput.Properties.ProvisioningState = helpers.ProvisioningStateSucceeded
	if err := azure.PutRPState(jobData.RPInput.SubscriptionId, jobData.RPInput.Id, jobData.RPInput.Properties); err != nil {
		jobData.RPInput.Properties.ProvisioningState = helpers.ProvisioningStateFailed
		responseError := helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to save RP state from put: %v", err))
		if err := azure.SetFailedProvisioningState(jobData.RPInput.SubscriptionId, jobData.RPInput.Id, responseError); err != nil {
			log.Debugf("Failed to Merge RP State for response error %v: %v", responseError, err)
		}
	}

	log.Debugf("Finished processing PUT request for %s", jobData.RPInput.Id)
}
