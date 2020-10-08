package helpers

import (
	"fmt"
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"
)

func ExecutePorterCommand(args []string) ([]byte, error) {
	if isDriverCommand(args[0]) {
		args = append(args, "--driver", "azure")
	}

	if isOutputCommand(args[0]) {
		args = append(args, "--output", "json")
	}

	log.Debugf("porter %v", args)
	out, err := exec.Command("porter", args...).CombinedOutput()
	if err != nil {
		log.Debugf("Command failed Error:%v Output: %s", err, string(out))
		return out, fmt.Errorf("Porter command failed: %v", err)
	}

	return out, nil
}

func isDriverCommand(cmd string) bool {
	return strings.Contains("installupgradeuninstallaction", cmd)
}

func isOutputCommand(cmd string) bool {
	return cmd == "installations"
}
