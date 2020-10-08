package jobs

import (
	log "github.com/sirupsen/logrus"
)

var MaxJobs int = 2

func Start() {
	log.Debug("Starting Jobs")
	startPutJob()
	startDeleteJob()
}

func Stop() {
	log.Debug("Stopping Jobs")
	close(PutJobs)
	close(DeleteJobs)
}
