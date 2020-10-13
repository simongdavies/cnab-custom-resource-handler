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

type PostJobData struct {
	RPInput          *models.BundleRP
	Args             []string
	InstallationName string
	OperationId      string
	Action           string
}

var PostJobs chan *PostJobData = make(chan *PostJobData, 20)

func startPostJob() {
	for i := 0; i < MaxJobs; i++ {
		go func(postJobs chan *PostJobData, i int) {
			log.Debugf("Starting Post Job %d", i)
			for jobData := range postJobs {
				log.Debugf("Starting Post Resource Job for %s", jobData.RPInput.Id)
				postJob(jobData)
				log.Debugf("Finished Post Resource Job for %s", jobData.RPInput.Id)
			}
			log.Debugf("Stopped Post Job %d", i)
		}(PostJobs, i)
	}
}

func postJob(data *PostJobData) {

	log.Debugf("Started processing POST request for %s", data.RPInput.Id)

	//TODO Implement Timeouts
	status := helpers.StatusFailed
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		responseError := fmt.Errorf("error creating temp dir: %v", err)
		updateStatus(data.RPInput, data.Action, status, data.OperationId, responseError)
		return
	}
	defer os.RemoveAll(dir)

	if len(data.RPInput.Properties.Parameters) > 0 {
		paramFile, err := common.WriteParametersFile(data.RPInput.Properties.Parameters, dir)
		if err != nil {
			updateStatus(data.RPInput, data.Action, status, data.OperationId, err)
			return
		}
		data.Args = append(data.Args, "-p", paramFile.Name())
		defer os.Remove(paramFile.Name())
	}

	if len(data.RPInput.Properties.Credentials) > 0 {
		credFile, err := common.WriteCredentialsFile(data.RPInput.Properties.Credentials, dir)
		if err != nil {
			updateStatus(data.RPInput, data.Action, status, data.OperationId, err)
			return
		}
		data.Args = append(data.Args, "-c", credFile.Name())
		defer os.Remove(credFile.Name())
	}

	out, err := helpers.ExecutePorterCommand(data.Args)
	if err == nil {
		status = helpers.StatusSucceeded
	}

	updateStatus(data.RPInput, data.Action, status, data.OperationId, string(out))

	log.Debugf("Finished processing POST request for %s", data.RPInput.Id)

}

func updateStatus(rpInput *models.BundleRP, action string, status string, operationId string, result interface{}) {
	// Always reset the RP status only ASyncOp will show final operation status
	rpInput.Properties.Status = ""
	if err := azure.PutRPState(rpInput.SubscriptionId, rpInput.Id, rpInput.Properties); err != nil {
		log.Debugf("Failed to update state:%v", err)
	}
	if err := azure.PutAsyncOp(rpInput.SubscriptionId, operationId, action, status, result); err != nil {
		log.Debugf("Failed to update Async Op for oeprationId %s: %v", operationId, err)
	}
}
