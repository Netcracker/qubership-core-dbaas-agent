package config

import (
	"context"
	"errors"

	"github.com/netcracker/qubership-core-lib-go/v3/logging"
)

var (
	logger logging.Logger
)

func init() {
	logger = logging.GetLogger("security-stub")
}

type ClassifierServiceName struct {
}

type ServiceNameProvider interface {
	GetServiceName(userCtx context.Context, classifier interface{}) (string, error)
}

func (_ ClassifierServiceName) GetServiceName(_ context.Context, classifier interface{}) (string, error) {
	logger.Info("Trying to get origin service name fromm classifier")
	if classifier == nil {
		return "", errors.New("request must contain a classifier")
	}

	if microserviceNameFromClassifier, found := classifier.(map[string]interface{})["microserviceName"]; found {
		return microserviceNameFromClassifier.(string), nil
	}
	return "", errors.New("request must contain microserviceName in a classifier")
}
