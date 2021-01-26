package jobs

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/simongdavies/cnab-custom-resource-handler/pkg/azure"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/common"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/helpers"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/models"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/settings"
	log "github.com/sirupsen/logrus"
)

type DeleteJobData struct {
	RPInput          *models.BundleRP
	Args             []string
	InstallationName string
	OperationId      string
	BundleInfo       *settings.BundleInformation
}

var DeleteJobs chan *DeleteJobData = make(chan *DeleteJobData, 20)

func startDeleteJob() {
	for i := 0; i < MaxJobs; i++ {
		go func(deleteJobs chan *DeleteJobData, i int) {
			log.Debugf("Starting Delete Job %d", i)
			for jobData := range deleteJobs {
				log.Debugf("Starting Delete Resource Job for %s", jobData.RPInput.Id)
				deleteJob(jobData)
				log.Debugf("Finished Delete Resource Job for %s", jobData.RPInput.Id)
			}
			log.Debugf("Stopped Delete Job %d", i)
		}(DeleteJobs, i)
	}
}

func deleteJob(jobData *DeleteJobData) {

	//TODO retry delete with last used Tag in case of errors
	log.Debugf("Started processing DELETE request for %s", jobData.RPInput.Id)
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

	properties, err := azure.GetRPState(jobData.RPInput.SubscriptionId, jobData.RPInput.Id)
	if err != nil {
		responseError := helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to get RPState for Delete: %v", err))
		if err := azure.SetFailedProvisioningState(jobData.RPInput.SubscriptionId, jobData.RPInput.Id, responseError); err != nil {
			log.Debugf("Failed to Merge RP State for response error %v: %v", responseError, err)
		}
		return
	}

	jobData.RPInput.Properties = properties

	jobData.Args = append(jobData.Args, "uninstall", jobData.InstallationName, "--delete", "--tag", jobData.BundleInfo.BundlePullOptions.Tag, "--force-delete")

	if len(jobData.RPInput.Properties.Parameters) > 0 {
		paramFile, err := common.WriteParametersFile(jobData.BundleInfo.RPBundle, jobData.RPInput.Properties.Parameters, dir)
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
		credFile, err := common.WriteCredentialsFile(jobData.BundleInfo.RPBundle, jobData.RPInput.Properties.Credentials, dir)
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

	out, err := helpers.ExecutePorterCommand(jobData.Args)
	if err != nil {
		responseError := helpers.ErrorInternalServerError(string(out))
		if err := azure.SetFailedProvisioningState(jobData.RPInput.SubscriptionId, jobData.RPInput.Id, responseError); err != nil {
			log.Debugf("Failed to Merge RP State for response error %v: %v", responseError, err)
		}
		return
	}

	if err := azure.DeleteRPState(jobData.RPInput.SubscriptionId, jobData.RPInput.Id); err != nil {
		responseError := helpers.ErrorInternalServerErrorFromError(fmt.Errorf("Failed to delete RP state for %s error: %v", jobData.RPInput.Id, err))
		if err := azure.SetFailedProvisioningState(jobData.RPInput.SubscriptionId, jobData.RPInput.Id, responseError); err != nil {
			log.Debugf("Failed to Delete RP State for response error %v: %v", responseError, err)
		}
	}

	if err := azure.PutAsyncOp(jobData.RPInput.SubscriptionId, jobData.OperationId, "delete", helpers.AsyncOperationComplete, nil); err != nil {
		log.Debugf("Failed to update async op for %s error: %v", jobData.RPInput.Id, err)
		return
	}

	log.Debugf("Finished processing DELETE request for %s", jobData.RPInput.Id)

}
