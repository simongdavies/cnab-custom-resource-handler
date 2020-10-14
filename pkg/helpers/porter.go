package helpers

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/simongdavies/cnab-custom-resource-handler/pkg/common"
	log "github.com/sirupsen/logrus"
)

type PorterOutput struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
	Type  string `json:"Type"`
}

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

func GetBundleOutput(installationName string, actions []string) ([]PorterOutput, error) {
	var cmdOutput []PorterOutput
	args := []string{}
	args = append(args, "installations", "output", "list", "-i", installationName)
	out, err := ExecutePorterCommand(args)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(out, &cmdOutput); err != nil {
		return nil, fmt.Errorf("Failed to read json from command: %v", err)
	}
	var actionOuput []PorterOutput
	for i, v := range cmdOutput {
		if isOutputForAnyAction(common.RPBundle.Outputs[v.Name].ApplyTo, actions) {
			actionOuput = append(actionOuput, cmdOutput[i])
		}

	}

	return actionOuput, nil
}
func isOutputForAnyAction(appliesTo []string, actions []string) bool {
	for i := 0; i < len(actions); i++ {
		for _, a := range appliesTo {
			if strings.EqualFold(a, actions[i]) {
				return true
			}
		}
	}
	return false
}
