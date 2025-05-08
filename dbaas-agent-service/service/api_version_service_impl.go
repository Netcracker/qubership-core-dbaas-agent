package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/apiversion"
	"github.com/valyala/fasthttp"
)

type apiVersionServiceImpl struct {
	apiVersionResponseCache *apiversion.ApiVersionResponse
	forwarder               *Forwarder
	service                 apiversion.ApiVersionService
}

func (apiVersionServiceImpl *apiVersionServiceImpl) GetApiVersion(userCtx context.Context) (*apiversion.ApiVersionResponse, error) {
	agentSpecs, err := apiVersionServiceImpl.service.GetApiVersion(userCtx)
	if err != nil {
		return nil, err
	}
	dbaasSpecs, err := getDbaasSpecs(userCtx, *apiVersionServiceImpl.forwarder)
	if err != nil {
		return nil, err
	}

	var resultSpecs apiversion.ApiVersionResponse
	for _, agentSpec := range agentSpecs.Specs {
		for _, dbaasSpec := range dbaasSpecs.Specs {
			if dbaasSpec.SpecRootUrl == agentSpec.SpecRootUrl {
				var resultSpec apiversion.Info
				resultSpec.SpecRootUrl = agentSpec.SpecRootUrl
				resultSpec.Major = agentSpec.Major
				resultSpec.Minor = agentSpec.Minor
				if *dbaasSpec.Minor < *agentSpec.Minor {
					resultSpec.Minor = dbaasSpec.Minor
				}
				supportedMajors := findCommonMajors(dbaasSpec.SupportedMajors, agentSpec.SupportedMajors)
				resultSpec.SupportedMajors = supportedMajors
				resultSpecs.Specs = append(resultSpecs.Specs, resultSpec)
			}
		}
	}
	return &resultSpecs, nil
}

func getDbaasSpecs(userCtx context.Context, forwarder Forwarder) (*apiversion.ApiVersionResponse, error) {
	resp, err := forwarder.DoRequest(userCtx, fasthttp.MethodGet, "/api-version", nil)
	defer fasthttp.ReleaseResponse(resp)
	if err != nil {
		logger.ErrorC(userCtx, "Error during forwarding request to dbaas-aggregator: %v", err)
		return nil, errors.New("Error happened during forwarding request to DBaaS")
	}
	var dbaasSpecs apiversion.ApiVersionResponse
	err = json.Unmarshal(resp.Body(), &dbaasSpecs)
	if err != nil {
		logger.ErrorC(userCtx, "Error happened during unmarshal DBaaS response: %v", err)
		return nil, errors.New("error happened during unmarshal DBaaS response")
	}
	return &dbaasSpecs, nil
}

func NewApiVersionService(config apiversion.ApiVersionConfig, forwarder *Forwarder) (apiversion.ApiVersionService, error) {
	service, err := apiversion.NewApiVersionService(config)
	if err != nil {
		return nil, err
	}
	return &apiVersionServiceImpl{forwarder: forwarder, service: service}, nil
}

func findCommonMajors(firstMajors, secondMajors []int) []int {
	var commonMajors []int
	for _, major := range firstMajors {
		for _, otherMajor := range secondMajors {
			if major == otherMajor {
				commonMajors = append(commonMajors, major)
			}
		}
	}
	return commonMajors
}
