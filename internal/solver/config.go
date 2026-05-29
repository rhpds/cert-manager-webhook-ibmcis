package solver

import (
	"encoding/json"
	"fmt"
	"regexp"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

var crnPattern = regexp.MustCompile(`^crn:v1:[a-zA-Z0-9-]*:.*$`)

type ibmcisConfig struct {
	CisCRNs []string `json:"cisCRN"`
}

func loadConfig(cfgJSON *extapi.JSON) (ibmcisConfig, error) {
	if cfgJSON == nil {
		return ibmcisConfig{}, fmt.Errorf("no configuration provided")
	}

	var cfg ibmcisConfig
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return ibmcisConfig{}, fmt.Errorf("decoding solver config: %w", err)
	}

	if len(cfg.CisCRNs) == 0 {
		return ibmcisConfig{}, fmt.Errorf("no CIS CRNs provided in configuration")
	}

	for _, crn := range cfg.CisCRNs {
		if !crnPattern.MatchString(crn) {
			return ibmcisConfig{}, fmt.Errorf("invalid CRN format: %s", crn)
		}
	}

	return cfg, nil
}
