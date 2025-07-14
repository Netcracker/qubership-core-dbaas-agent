package controller

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/config"
	"os"

	"github.com/gofiber/fiber/v2"
	_ "github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/domain"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/service"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/baseproviders/tenant"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/valyala/fasthttp"
)

const (
	TenantHeader                     = "Tenant"
	MsgErrorRequest                  = "Error during forwarding request to dbaas-aggregator: %v"
	MsgErrorForwardingRequestToDbaas = "Error happened during forwarding request to DBaaS"
	MsgReceivedRequest               = "Received %v request to %v"
	MsgErrorTenantContext            = "Got error during work with tenant context"
	MsgNotAllowedToAccessTenant      = "you are not allowed to access this tenant"
	MsgNotAllowedToAccessNamespace   = "You are not allowed to access this namespace"
	MsgOwner                         = "M2M token must contains owner field"
	Owner                            = "owner"
)

var (
	logger logging.Logger
)

// @title Dbaas Agent API
// @description DbaaS Agent is a microservice deployed inside functional project to get access to DbaaS Aggregator service. For more information, visit our Documentation (https://github.com/netcracker/qubership-core-dbaas-agent/blob/main/README.md).
// @version 2.0
// @tag.name API V3
// @tag.description Apis of DB activities related to V3
// @Produce json
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
func init() {
	logger = logging.GetLogger("controller")
}

type Controller struct {
	securityService *service.SecurityService
	forwarder       *service.Forwarder
}

func NewController(securityService *service.SecurityService, forwarder *service.Forwarder) *Controller {
	return &Controller{securityService: securityService, forwarder: forwarder}
}

// @Summary Get Or Create Database
// @Description Get Or Create Database
// @Tags API V3
// @ID GetOrCreateDatabaseV3
// @Produce  json
// @Param	request 		body     map[string]interface{}  true   "request body"
// @Param	namespace		path     string     				 true   "namespace"
// @Security ApiKeyAuth
// @Success 200 {object}    []byte
// @Success 201 {object}    []byte
// @Failure 401 {object}	map[string]string
// @Failure 403 {object}	map[string]string
// @Failure 400 {object}    map[string]string
// @Failure 500 {object}	map[string]string
// @Router /api/v3/dbaas/{namespace}/databases [put]
func (controller *Controller) HandleGetOrCreateDatabaseV3(c *fiber.Ctx) error {
	userCtx := c.UserContext()
	logger.InfoC(userCtx, MsgReceivedRequest, c.Method(), c.Path())

	body, _, err := readRequestBody(userCtx, c)
	if err != nil {
		logger.ErrorC(userCtx, "Failed to read request body in create database handler: %v", err)
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, "Failed to read request body in create database handler")
	}
	if body == nil {
		logger.ErrorC(userCtx, "Request body in create database handler is nil")
		return respondWithError(userCtx, c, fiber.StatusBadRequest, "request body in create database handler is nil")
	}

	logger.InfoC(userCtx, "Request to create %v database from %v project with classifier %v", body["type"],
		c.Params("namespace"), body["classifier"])

	tenantObject, err := tenant.Of(userCtx)
	if err != nil {
		logger.ErrorC(userCtx, MsgErrorTenantContext+" : %+v", err)
		return respondWithError(userCtx, c, fiber.StatusForbidden, MsgErrorTenantContext)
	}
	if controller.securityService.CheckTenantId(userCtx, body, tenantObject.GetTenant()) != nil {
		return respondWithError(userCtx, c, fiber.StatusForbidden, MsgNotAllowedToAccessTenant)
	}

	namespaceFromPath := c.Params("namespace")
	if controller.securityService.CheckNamespaceIsolation(userCtx, namespaceFromPath) != nil {
		return respondWithError(userCtx, c, fiber.StatusForbidden, MsgNotAllowedToAccessNamespace)
	}

	err, errCode := controller.validateNamespaceFromClassifier(userCtx, body["classifier"])
	if err != nil {
		return respondWithError(userCtx, c, errCode, err.Error())
	}

	serviceNameProvider := serviceloader.MustLoad[config.ServiceNameProvider]()
	serviceName, err1 := serviceNameProvider.GetServiceName(userCtx, body["classifier"])
	if err1 != nil {
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, err1.Error())
	}

	if len(serviceName) != 0 {
		body["originService"] = serviceName
	} else {
		return respondWithError(userCtx, c, fiber.StatusBadRequest, MsgOwner)
	}

	enrichedBody, err := json.Marshal(body)
	if err != nil {
		logger.ErrorC(userCtx, "Failed to marshal enriched request body: %v; error: %v", enrichedBody, err)
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, "Failed to add owner field to forwarding database request")
	}
	logger.DebugC(userCtx, "Enriched request body: %s", string(enrichedBody))

	resp, err := controller.forwarder.DoRequest(userCtx, c.Method(), c.Path(), enrichedBody)
	defer fasthttp.ReleaseResponse(resp)
	if err != nil {
		logger.ErrorC(userCtx, MsgErrorRequest, err)
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, MsgErrorForwardingRequestToDbaas)
	}
	return returnDbaasResponse(userCtx, c, resp)
}

func (controller *Controller) validateNamespaceFromClassifier(userCtx context.Context, classifier interface{}) (error, int) {
	namespaceFromClassifier, err := GetNamespaceFromClassifier(classifier)
	if err != nil {
		return err, fiber.StatusBadRequest
	}
	if err := controller.securityService.CheckNamespaceFromClassifier(userCtx, namespaceFromClassifier); err != nil {
		return err, fiber.StatusForbidden
	}
	return nil, 0
}

func GetNamespaceFromClassifier(classifier interface{}) (string, error) {
	if classifier == nil {
		return "", errors.New("request must contain a classifier")
	}
	namespace, ok := classifier.(map[string]interface{})["namespace"].(string)
	if !ok {
		return "", errors.New("request must contain namespace in a classifier")
	}
	return namespace, nil
}

// @Summary Getting Connection By Classifier
// @Description Getting Connection By Classifier
// @Tags API V3
// @ID GettingConnectionByClassifierV3
// @Produce  json
// @Param	request 		body     map[string]interface{}    true   "ClassifierWithRolesRequest"
// @Param	namespace		path     string     				 true   "namespace"
// @Param	type		    path     string     				 true   "type"
// @Security ApiKeyAuth
// @Success 200 {object}    []byte
// @Failure 403 {object}	map[string]string
// @Failure 404 {object}    map[string]string
// @Failure 500 {object}	map[string]string
// @Router /api/v3/dbaas/{namespace}/databases/get-by-classifier/{type} [post]
func (controller *Controller) HandleGettingConnectionByClassifierV3(c *fiber.Ctx) error {
	userCtx := c.UserContext()
	logger.InfoC(userCtx, MsgReceivedRequest, c.Method(), c.Path())

	body, _, err := readRequestBody(userCtx, c)
	if err != nil {
		logger.ErrorC(userCtx, "Failed to read request body in get database by classifier handler: %v", err)
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, "Failed to read request body in get database by classifier handler")
	}
	tenantObject, err := tenant.Of(userCtx)
	if err != nil {
		logger.ErrorC(userCtx, MsgErrorTenantContext+" : %+v", err)
		return respondWithError(userCtx, c, fiber.StatusForbidden, MsgErrorTenantContext)
	}
	if controller.securityService.CheckTenantId(userCtx, body, tenantObject.GetTenant()) != nil {
		return respondWithError(userCtx, c, fiber.StatusForbidden, MsgNotAllowedToAccessTenant)
	}

	namespaceFromPath := c.Params("namespace")
	if controller.securityService.CheckNamespaceIsolation(userCtx, namespaceFromPath) != nil {
		return respondWithError(userCtx, c, fiber.StatusForbidden, MsgNotAllowedToAccessNamespace)
	}
	err, errCode := controller.validateNamespaceFromClassifier(userCtx, body["classifier"])
	if err != nil {
		return respondWithError(userCtx, c, errCode, err.Error())
	}

	serviceNameProvider := serviceloader.MustLoad[config.ServiceNameProvider]()
	serviceName, err := serviceNameProvider.GetServiceName(userCtx, body["classifier"])
	if err != nil {
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, err.Error())
	}

	if len(serviceName) != 0 {
		body["originService"] = serviceName
	} else {
		return respondWithError(userCtx, c, fiber.StatusBadRequest, MsgOwner)
	}

	enrichedBody, err := json.Marshal(body)
	if err != nil {
		logger.ErrorC(userCtx, "Failed to marshal enriched request body in get database by classifier handler: %v", err)
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, "Failed to create new request body in get database by classifier handler")
	}
	resp, err := controller.forwarder.DoRequest(userCtx, c.Method(), c.Path(), enrichedBody)
	defer fasthttp.ReleaseResponse(resp)
	if err != nil {
		logger.ErrorC(userCtx, MsgErrorRequest, err)
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, MsgErrorForwardingRequestToDbaas)
	}
	return returnDbaasResponse(userCtx, c, resp)
}

// @Summary Getting Physical Databases
// @Description Getting Physical Databases
// @Tags API V3
// @ID GettingPhysicalDatabases
// @Produce  json
// @Param	type		    path     string     				 true   "type"
// @Security ApiKeyAuth
// @Success 200 {object}    []byte
// @Failure 404 {object}    map[string]string
// @Router /api/v3/dbaas/{type}/physical_databases [get]
func (controller *Controller) HandleGettingPhysicalDatabases(ctx *fiber.Ctx) error {
	userContext := ctx.UserContext()

	dbType := ctx.Params("type")
	logger.InfoC(userContext, "Request to get all available physical databases of type %v", dbType)

	resp, err := controller.forwarder.DoRequest(userContext, ctx.Method(), ctx.Path(), nil)
	defer fasthttp.ReleaseResponse(resp)
	if err != nil {
		logger.ErrorC(userContext, MsgErrorRequest, err)
		return respondWithError(userContext, ctx, fiber.StatusInternalServerError, MsgErrorForwardingRequestToDbaas)
	}
	return returnDbaasResponse(userContext, ctx, resp)
}

// @Summary Deletion By Classifier
// @Description Deletion By Classifier
// @Tags API V3
// @ID DeletionByClassifier
// @Produce  json
// @Param	request 		body     map[string]interface{}    true   "ClassifierWithRolesRequest"
// @Param	namespace		path     string     				 true   "namespace"
// @Param	type		    path     string     				 true   "type"
// @Security ApiKeyAuth
// @Success 200 {object}    []byte
// @Success 201 {object}    []byte
// @Failure 403 {object}	map[string]string
// @Failure 404 {object}    map[string]string
// @Failure 500 {object}	map[string]string
// @Router /api/v3/dbaas/{namespace}/databases/{type} [delete]
func (controller *Controller) HandleDeletionByClassifier(c *fiber.Ctx) error {
	userCtx := c.UserContext()
	logger.InfoC(userCtx, MsgReceivedRequest, c.Method(), c.Path())

	body, _, err := readRequestBody(userCtx, c)
	if err != nil {
		logger.ErrorC(userCtx, "Failed to read request body in delete database by classifier handler: %v", err)
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, err.Error())
	}

	serviceNameProvider := serviceloader.MustLoad[config.ServiceNameProvider]()
	serviceName, er := serviceNameProvider.GetServiceName(userCtx, body["classifier"])
	if er != nil {
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, er.Error())
	}

	if len(serviceName) != 0 {
		body["originService"] = serviceName
	} else {
		return respondWithError(userCtx, c, fiber.StatusBadRequest, MsgOwner)
	}

	enrichedBody, err := json.Marshal(body)
	if err != nil {
		logger.ErrorC(userCtx, "Failed to marshal enriched request body in delete database by classifier handler: %v", err)
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, "Failed to create new request body in delete database by classifier handler")
	}

	namespaceFromPath := c.Params("namespace")
	if controller.securityService.CheckNamespaceIsolation(userCtx, namespaceFromPath) != nil {
		return respondWithError(userCtx, c, fiber.StatusForbidden, MsgNotAllowedToAccessNamespace)
	}

	resp, err := controller.forwarder.DoRequest(userCtx, c.Method(), c.Path(), enrichedBody)
	defer fasthttp.ReleaseResponse(resp)
	if err != nil {
		logger.ErrorC(userCtx, MsgErrorRequest, err)
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, MsgErrorForwardingRequestToDbaas)
	}
	return returnDbaasResponse(userCtx, c, resp)
}

// @Summary Registration Externally Manageable DB
// @Description Registration Externally Manageable DB
// @Tags API V3
// @ID RegistrationExternallyManageableDBV3
// @Produce  json
// @Param	namespace		    path     string     				 true   "namespace"
// @Param	request 		    body     map[string]interface{}    true   "ClassifierWithRolesRequest"
// @Security ApiKeyAuth
// @Success 200 {object}    []byte
// @Failure 401 {object}	map[string]string
// @Failure 403 {object}	map[string]string
// @Failure 409 {object}    map[string]string
// @Failure 500 {object}	map[string]string
// @Router /api/v3/dbaas/{namespace}/databases/registration/externally_manageable [put]
func (controller *Controller) HandleRegistrationExternallyManageableDBV3(c *fiber.Ctx) error {
	userCtx := c.UserContext()
	logger.InfoC(userCtx, MsgReceivedRequest, c.Method(), c.Path())

	body, _, err := readRequestBody(userCtx, c)
	if err != nil {
		logger.ErrorC(userCtx, "Failed to read request body in registration externally manageable handler: %v", err)
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, "Failed to read request body in registration externally manageable handler")
	}

	tenantObject, err := tenant.Of(userCtx)
	if err != nil {
		logger.ErrorC(userCtx, MsgErrorTenantContext+" : %+v", err)
		return respondWithError(userCtx, c, fiber.StatusForbidden, MsgErrorTenantContext)
	}
	if tenantObject.GetTenant() != "" && controller.securityService.CheckTenantId(userCtx, body, tenantObject.GetTenant()) != nil {
		return respondWithError(userCtx, c, fiber.StatusForbidden, MsgNotAllowedToAccessTenant)
	}

	namespaceFromPath := c.Params("namespace")
	if controller.securityService.CheckNamespaceIsolation(userCtx, namespaceFromPath) != nil {
		return respondWithError(userCtx, c, fiber.StatusForbidden, MsgNotAllowedToAccessNamespace)
	}
	err, errCode := controller.validateNamespaceFromClassifier(userCtx, body["classifier"])
	if err != nil {
		return respondWithError(userCtx, c, errCode, err.Error())
	}

	serviceNameProvider := serviceloader.MustLoad[config.ServiceNameProvider]()
	serviceName, erro := serviceNameProvider.GetServiceName(userCtx, body["classifier"])
	if erro != nil {
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, erro.Error())
	}

	if len(serviceName) != 0 {
		body["originService"] = serviceName
	} else {
		return respondWithError(userCtx, c, fiber.StatusBadRequest, MsgOwner)
	}

	enrichedBody, err := json.Marshal(body)
	if err != nil {
		logger.ErrorC(userCtx, "Failed to marshal enriched request body in get database by classifier handler: %v", err)
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, "Failed to create new request body in get database by classifier handler")
	}

	resp, err := controller.forwarder.DoRequest(userCtx, c.Method(), c.Path(), enrichedBody)
	defer fasthttp.ReleaseResponse(resp)
	if err != nil {
		logger.ErrorC(userCtx, MsgErrorRequest, err)
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, MsgErrorForwardingRequestToDbaas)
	}
	logger.DebugC(userCtx, "response from dbaas: %v", resp)

	if resp.StatusCode() == fasthttp.StatusNotFound {
		logger.WarnC(userCtx, "Check dbaas-aggregator version, which must be 3.12.0 or higher")
		return respondWithError(userCtx, c, resp.StatusCode(), "Not Found. Check dbaas-aggregator version, which must be 3.12.0 or higher")
	}
	return returnDbaasResponse(userCtx, c, resp)
}

// @Summary Getting all databases by namespace
// @Description List of all databases
// @Tags API V3
// @ID GettingAllDatabasesByNamespaceV3
// @Produce  json
// @Param	namespace		path     string     			   true   "namespace"
// @Param	withResources   query    bool     				   false  "withResources"
// @Security ApiKeyAuth
// @Success 200 {object}    []byte
// @Failure 403 {object}	map[string]string
// @Failure 500 {object}	map[string]string
// @Router /api/v3/dbaas/{namespace}/databases/list [get]
func (controller *Controller) HandleGettingAllDatabasesByNamespaceV3(c *fiber.Ctx) error {
	userCtx := c.UserContext()
	logger.InfoC(userCtx, MsgReceivedRequest, c.Method(), c.Path())

	namespaceFromPath := c.Params("namespace")
	if controller.securityService.CheckNamespaceIsolation(userCtx, namespaceFromPath) != nil {
		return respondWithError(userCtx, c, fiber.StatusForbidden, MsgNotAllowedToAccessNamespace)
	}

	resp, err := controller.forwarder.DoRequest(userCtx, c.Method(), c.Path(), nil)
	defer fasthttp.ReleaseResponse(resp)
	if err != nil {
		logger.ErrorC(userCtx, MsgErrorRequest, err)
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, MsgErrorForwardingRequestToDbaas)
	}
	return returnDbaasResponse(userCtx, c, resp)
}

func (controller *Controller) ForwardHandler(c *fiber.Ctx) error {
	userCtx := c.UserContext()
	logger.InfoC(userCtx, MsgReceivedRequest, c.Method(), c.Path())
	body := c.Request().Body()

	resp, err := controller.forwarder.DoRequest(userCtx, c.Method(), c.Path(), body)
	defer fasthttp.ReleaseResponse(resp)
	if err != nil {
		logger.ErrorC(userCtx, MsgErrorRequest, err)
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, MsgErrorForwardingRequestToDbaas)
	}
	logger.DebugC(userCtx, "response from dbaas: %v", resp)
	return returnDbaasResponse(userCtx, c, resp)
}

func (controller *Controller) HandleSimpleGet(ctx *fiber.Ctx) error {
	userCtx := ctx.UserContext()
	resp, err := controller.forwarder.DoRequest(userCtx, ctx.Method(), ctx.Path(), nil)
	defer fasthttp.ReleaseResponse(resp)
	if err != nil {
		logger.ErrorC(userCtx, "Error during forwarding request to dbaas-aggregator: %v", err)
		return respondWithError(userCtx, ctx, fiber.StatusInternalServerError, "Error happened during forwarding request to DBaaS")
	}
	return returnDbaasResponse(userCtx, ctx, resp)
}

func LoadConfigParameter(file, envName string) string {
	buf, err := os.ReadFile(file)
	if err == nil {
		return string(buf)
	} else {
		logger.Error("Error loading configuration parameter from file", err)
		return configloader.GetOrDefaultString(envName, "")
	}
}
