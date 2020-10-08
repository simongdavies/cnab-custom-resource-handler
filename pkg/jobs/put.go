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

func putJob(data *PutJobData) {

	log.Debugf("Started processing PUT request for %s", data.RPInput.Id)

	//TODO Implement Timeouts

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		responseError := helpers.ErrorInternalServerErrorFromError(fmt.Errorf("error creating temp dir: %v", err))
		if err := azure.MergeRPState(data.RPInput.SubscriptionId, data.RPInput.Id, responseError); err != nil {
			log.Debugf("Failed to Merge RP State for response error %v: %v", responseError, err)
		}
		return
	}
	defer os.RemoveAll(dir)

	if len(data.RPInput.Properties.Parameters) > 0 {
		paramFile, err := common.WriteParametersFile(data.RPInput.Properties.Parameters, dir)
		if err != nil {
			responseError := helpers.ErrorInternalServerErrorFromError(err)
			if err := azure.MergeRPState(data.RPInput.SubscriptionId, data.RPInput.Id, responseError); err != nil {
				log.Debugf("Failed to Merge RP State for response error %v: %v", responseError, err)
			}
			return
		}
		data.Args = append(data.Args, "-p", paramFile.Name())
		defer os.Remove(paramFile.Name())
	}

	if len(data.RPInput.Properties.Credentials) > 0 {
		credFile, err := common.WriteCredentialsFile(data.RPInput.Properties.Credentials, dir)
		if err != nil {
			responseError := helpers.ErrorInternalServerErrorFromError(err)
			if err := azure.MergeRPState(data.RPInput.SubscriptionId, data.RPInput.Id, responseError); err != nil {
				log.Debugf("Failed to Merge RP State for response error %v: %v", responseError, err)
			}
			return
		}
		data.Args = append(data.Args, "-c", credFile.Name())
		defer os.Remove(credFile.Name())
	}

	if out, err := helpers.ExecutePorterCommand(data.Args); err != nil {
		responseError := helpers.ErrorInternalServerError(string(out))
		if err := azure.MergeRPState(data.RPInput.SubscriptionId, data.RPInput.Id, responseError); err != nil {
			log.Debugf("Failed to Merge RP State for response error %v: %v", responseError, err)
		}
		return
	}
	data.RPInput.Properties.ProvisioningState = helpers.ProvisioningStateSucceeded
	if err := azure.PutRPState(data.RPInput.SubscriptionId, data.RPInput.Id, data.RPInput.Properties); err != nil {
		responseError := helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to save RP state from put: %v", err))
		if err := azure.MergeRPState(data.RPInput.SubscriptionId, data.RPInput.Id, responseError); err != nil {
			log.Debugf("Failed to Merge RP State for response error %v: %v", responseError, err)
		}
	}

	log.Debugf("Finished processing PUT request for %s", data.RPInput.Id)
}
