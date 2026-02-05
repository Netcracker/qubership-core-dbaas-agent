package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/client"
	_ "github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/config"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/service"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/testutils"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/apiversion"
	fiberserver "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	constants "github.com/netcracker/qubership-core-lib-go/v3/const"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/valyala/fasthttp"
)

var (
	AuthorizationContextName = "authorization"
	PolicyFileProperty       = "policy.file.name"
)

type TestSuite struct {
	suite.Suite
	controller *Controller
	namespace  string
}

func (suite *TestSuite) SetupSuite() {
	testutils.StartMockServer()
	os.Setenv(constants.MicroserviceNameProperty, "test-name")
	os.Setenv(PolicyFileProperty, "../testutils/test-policies.conf")
	os.Setenv("secret.path", "../testutils/")
	os.Setenv(constants.ConfigServerUrlProperty, testutils.GetMockServerUrl())
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	testCreds := service.NewBasicCreds("test-user", []byte("test-pass"))
	forwarder := service.NewForwarder(testutils.GetMockServerUrl(), testCreds, testutils.GetMockServerClient())
	suite.namespace = "test-namespace"
	securityService := service.NewSecurityService("", false, suite.namespace, nil, nil)
	suite.controller = NewController(securityService, forwarder)
}

func (suite *TestSuite) TearDownSuite() {
	os.Unsetenv(PolicyFileProperty)
	os.Unsetenv(constants.ConfigServerUrlProperty)
	os.Unsetenv(constants.MicroserviceNameProperty)
	testutils.StopMockServer()
}

func (suite *TestSuite) BeforeTest(suiteName, testName string) {
	suite.T().Cleanup(testutils.ClearHandlers)
	testutils.AddMockCertsEndpointHandler()
	testutils.AddHandler(testutils.Contains("/test-name/default"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})
}

func TestExampleTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}

func (suite *TestSuite) TestController_HandleGettingConnectionByClassifierV3() {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	testRespBody, _ := json.Marshal(map[string]interface{}{
		"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"},
	})
	testutils.AddHandler(testutils.Contains("/get-by-classifier"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})

	app, err := fiberserver.New().Process()
	testToken := testutils.GetSignedTokenWithClaims("test", []string{"ROLE_TEST_ROLE"}, map[string]interface{}{"owner": "test-service"})
	assert.Nil(suite.T(), err)
	app.Post("/get-by-classifier", suite.controller.HandleGettingConnectionByClassifierV3)
	jsonStr := testRespBody
	req, _ := http.NewRequest(http.MethodPost, "/get-by-classifier", bytes.NewBuffer(jsonStr))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	time.Sleep(5 * time.Second)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_ErrorReadBody_HandleGettingConnectionByClassifierV3() {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	testRespBody, _ := json.Marshal(map[string]interface{}{"key": "val"})
	testutils.AddHandler(testutils.Contains("/get-by-classifier"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})

	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Post("/get-by-classifier", suite.controller.HandleGettingConnectionByClassifierV3)
	jsonStr := testRespBody
	req, _ := http.NewRequest(http.MethodPost, "/get-by-classifier", bytes.NewBuffer(jsonStr))

	time.Sleep(5 * time.Second)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusInternalServerError, response.StatusCode)
}

func (suite *TestSuite) TestController_HandleDeletionByClassifierV3() {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	testRespBody, _ := json.Marshal(map[string]interface{}{"classifier": map[string]interface{}{"key": "val", "clientId": "test-service", "microserviceName": "test-service"}})

	testutils.AddHandler(testutils.Contains("/databases"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})

	app, err := fiberserver.New().Process()
	testToken := testutils.GetSignedTokenWithClaims("test", []string{"ROLE_TEST_ROLE"}, map[string]interface{}{"owner": "test-service"})
	assert.Nil(suite.T(), err)
	app.Delete("/databases", suite.controller.HandleDeletionByClassifier)
	jsonStr := testRespBody
	req, _ := http.NewRequest(http.MethodDelete, "/databases", bytes.NewBuffer(jsonStr))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleDeletionByClassifierV3_WrongBodyContentType() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Delete("/databases", suite.controller.HandleDeletionByClassifier)

	badBody := []byte("bad_body_non_xml_content")
	req, _ := http.NewRequest(http.MethodDelete, "/databases", bytes.NewBuffer(badBody))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationXML)

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": "failed to unmarshal: EOF",
	})
	assert.Equal(suite.T(), http.StatusInternalServerError, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleDeletionByClassifierV3_NoOwner() {

	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Delete("/databases", suite.controller.HandleDeletionByClassifier)

	body, _ := json.Marshal(map[string]interface{}{"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"}})

	testToken := testutils.GetSignedTokenWithClaims("test", nil, nil)

	testRespBody, _ := json.Marshal(map[string]interface{}{"clientId": ""})
	testutils.AddHandler(testutils.Contains("/databases"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})

	req, _ := http.NewRequest(http.MethodDelete, "/databases", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleDeletionByClassifierV3_ForwardingError() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Delete("/databases", suite.controller.HandleDeletionByClassifier)
	testutils.AddHandler(testutils.Contains("/databases"), nil)

	body, _ := json.Marshal(map[string]interface{}{"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"}})

	testToken := testutils.GetSignedTokenWithClaims("test", nil, map[string]interface{}{"owner": "test-service"})
	req, _ := http.NewRequest(http.MethodDelete, "/databases", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": MsgErrorForwardingRequestToDbaas,
	})
	assert.Equal(suite.T(), http.StatusInternalServerError, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleDeletionByClassifierV3_NamespaceError() {
	testCreds := service.NewBasicCreds("test-user", []byte("test-pass"))
	forwarder := service.NewForwarder(testutils.GetMockServerUrl(), testCreds, testutils.GetMockServerClient())
	namespace := "test-namespace"
	restClient := GetMockRestClient(serialize(client.CompositeStructure{Baseline: "ns-1", Satellites: []string{"ns-2"}}), http.StatusOK, nil)
	controlPlaneClient := client.NewControlPlaneClient(restClient)
	securityService := service.NewSecurityService("", true, namespace, controlPlaneClient, nil)
	controller := NewController(securityService, forwarder)

	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Delete("/databases", controller.HandleDeletionByClassifier)

	body, _ := json.Marshal(map[string]interface{}{"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"}})
	testToken := testutils.GetSignedTokenWithClaims("test", nil, map[string]interface{}{"owner": "test-service"})
	req, _ := http.NewRequest(http.MethodDelete, "/databases", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": MsgNotAllowedToAccessNamespace,
	})
	assert.Equal(suite.T(), http.StatusForbidden, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGetOrCreateDatabaseV3() {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	testRespBody, _ := json.Marshal(map[string]interface{}{
		"namespace":  "test-namespace",
		"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"},
	})
	testutils.AddHandler(testutils.Contains("/databases"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})

	app, err := fiberserver.New().Process()
	testToken := testutils.GetSignedTokenWithClaims("test", []string{"ROLE_TEST_ROLE"}, map[string]interface{}{"owner": "test-service"})
	assert.Nil(suite.T(), err)
	app.Put("/databases", suite.controller.HandleGetOrCreateDatabaseV3)
	jsonStr := testRespBody
	req, _ := http.NewRequest(http.MethodPut, "/databases", bytes.NewBuffer(jsonStr))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	time.Sleep(5 * time.Second)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleRegistrationExternallyManageableDBV3() {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	testRespBody, _ := json.Marshal(map[string]interface{}{"key": "val", "classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"}})
	testutils.AddHandler(testutils.Contains("/registration/externally_manageable"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})

	app, err := fiberserver.New().Process()
	testToken := testutils.GetSignedTokenWithClaims("test", []string{"ROLE_TEST_ROLE"}, map[string]interface{}{"owner": "test-service"})
	assert.Nil(suite.T(), err)
	app.Put("/registration/externally_manageable", suite.controller.HandleRegistrationExternallyManageableDBV3)
	jsonStr := testRespBody
	req, _ := http.NewRequest(http.MethodPut, "/registration/externally_manageable", bytes.NewBuffer(jsonStr))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	time.Sleep(5 * time.Second)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_ErrorTenant_HandleRegistrationExternallyManageableDBV3() {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	testRespBody, _ := json.Marshal(map[string]interface{}{
		"tenantId":   "different",
		"namespace":  "test-namespace",
		"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"},
	})
	testutils.AddHandler(testutils.Contains("/registration/externally_manageable"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})

	app, err := fiberserver.New().Process()
	testToken := testutils.GetSignedTokenWithClaims("test", []string{"ROLE_TEST_ROLE"}, map[string]interface{}{"clientId": "test-service"})
	assert.Nil(suite.T(), err)
	app.Put("/registration/externally_manageable", suite.controller.HandleRegistrationExternallyManageableDBV3)
	jsonStr := testRespBody
	req, _ := http.NewRequest(http.MethodPut, "/registration/externally_manageable", bytes.NewBuffer(jsonStr))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	time.Sleep(5 * time.Second)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusOK, response.StatusCode)
}

func (suite *TestSuite) TestController_HandleRegistrationExternallyManageableDBV3_WrongBodyContentType() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Put("/registration/externally_manageable", suite.controller.HandleRegistrationExternallyManageableDBV3)

	badBody := []byte("bad_body_non_xml_content")
	testToken := testutils.GetSignedTokenWithClaims("test", []string{"ROLE_TEST_ROLE"}, map[string]interface{}{"owner": "test-service"})
	req, _ := http.NewRequest(http.MethodPut, "/registration/externally_manageable", bytes.NewBuffer(badBody))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationXML)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": "Failed to read request body in registration externally manageable handler",
	})
	assert.Equal(suite.T(), http.StatusInternalServerError, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleRegistrationExternallyManageableDBV3_NoNamespaceInClassifier() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Put("/registration/externally_manageable", suite.controller.HandleRegistrationExternallyManageableDBV3)

	body, _ := json.Marshal(map[string]interface{}{"classifier": map[string]interface{}{"microserviceName": "test-service"}})
	req, _ := http.NewRequest(http.MethodPut, "/registration/externally_manageable", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": "request must contain namespace in a classifier",
	})
	assert.Equal(suite.T(), http.StatusBadRequest, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleRegistrationExternallyManageableDBV3_NoOwner() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Put("/registration/externally_manageable", suite.controller.HandleRegistrationExternallyManageableDBV3)

	body, _ := json.Marshal(map[string]interface{}{"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"}})
	testToken := testutils.GetSignedTokenWithClaims("test", nil, nil)
	testRespBody, _ := json.Marshal(map[string]interface{}{"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"}})
	testutils.AddHandler(testutils.Contains("/registration/externally_manageable"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})

	req, _ := http.NewRequest(http.MethodPut, "/registration/externally_manageable", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleRegistrationExternallyManageableDBV3_ForwardingError() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Put("/registration/externally_manageable", suite.controller.HandleRegistrationExternallyManageableDBV3)
	testutils.AddHandler(testutils.Contains("/registration/externally_manageable"), nil)

	body, _ := json.Marshal(map[string]interface{}{"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"}})
	testToken := testutils.GetSignedTokenWithClaims("test", nil, map[string]interface{}{"owner": "test-service"})
	req, _ := http.NewRequest(http.MethodPut, "/registration/externally_manageable", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": MsgErrorForwardingRequestToDbaas,
	})
	assert.Equal(suite.T(), http.StatusInternalServerError, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleRegistrationExternallyManageableDBV3_NotFound() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Put("/registration/externally_manageable", suite.controller.HandleRegistrationExternallyManageableDBV3)
	testutils.AddHandler(testutils.Contains("/registration/externally_manageable"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
	})

	body, _ := json.Marshal(map[string]interface{}{"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"}})
	testToken := testutils.GetSignedTokenWithClaims("test", nil, map[string]interface{}{"owner": "test-service"})
	req, _ := http.NewRequest(http.MethodPut, "/registration/externally_manageable", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": "Not Found. Check dbaas-aggregator version, which must be 3.12.0 or higher",
	})
	assert.Equal(suite.T(), http.StatusNotFound, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleRegistrationExternallyManageableDBV3_NamespaceError() {
	testCreds := service.NewBasicCreds("test-user", []byte("test-pass"))
	forwarder := service.NewForwarder(testutils.GetMockServerUrl(), testCreds, testutils.GetMockServerClient())
	namespace := "test-namespace"
	restClient := GetMockRestClient(serialize(client.CompositeStructure{Baseline: "ns-1", Satellites: []string{"ns-2"}}), http.StatusOK, nil)
	controlPlaneClient := client.NewControlPlaneClient(restClient)
	securityService := service.NewSecurityService("", true, namespace, controlPlaneClient, nil)
	controller := NewController(securityService, forwarder)

	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Put("/registration/externally_manageable", controller.HandleRegistrationExternallyManageableDBV3)

	body, _ := json.Marshal(map[string]interface{}{"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"}})
	req, _ := http.NewRequest(http.MethodPut, "/registration/externally_manageable", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": MsgNotAllowedToAccessNamespace,
	})
	assert.Equal(suite.T(), http.StatusForbidden, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGetApiVersion() {
	testCreds := service.NewBasicCreds("test-user", []byte("test-pass"))
	forwarder := service.NewForwarder(testutils.GetMockServerUrl(), testCreds, testutils.GetMockServerClient())
	os.Setenv("API-VERSION_PATH", "../api-version-info.json")
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	data := map[string]interface{}{
		"specs": []interface{}{
			map[string]interface{}{
				"specRootUrl":     "/api",
				"major":           3,
				"minor":           14,
				"supportedMajors": []int{3},
			},
			map[string]interface{}{
				"specRootUrl":     "/api/bluegreen",
				"major":           1,
				"minor":           3,
				"supportedMajors": []int{1},
			},
			map[string]interface{}{
				"specRootUrl":     "/api/declarations",
				"major":           1,
				"minor":           0,
				"supportedMajors": []int{1},
			},
		},
	}

	testRespBody, _ := json.Marshal(data)
	testutils.AddHandler(testutils.Contains("/api-version"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})
	apiVersionService, _ := service.NewApiVersionService(apiversion.ApiVersionConfig{}, forwarder)
	app, err := fiberserver.New().
		WithApiVersion(apiVersionService).
		Process()
	assert.Nil(suite.T(), err)
	req, _ := http.NewRequest(http.MethodGet, "/api-version", nil)

	time.Sleep(5 * time.Second)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)
	var respSpecs apiversion.ApiVersionResponse
	_ = json.Unmarshal(respBody, &respSpecs)
	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
	assert.NotNil(suite.T(), respSpecs.Specs)
}

func (suite *TestSuite) TestController_HandleGetApiVersion_ForwardingError() {
	testCreds := service.NewBasicCreds("test-user", []byte("test-pass"))
	forwarder := service.NewForwarder(testutils.GetMockServerUrl(), testCreds, testutils.GetMockServerClient())
	apiVersionService, _ := service.NewApiVersionService(apiversion.ApiVersionConfig{}, forwarder)
	app, err := fiberserver.New().
		WithApiVersion(apiVersionService).
		Process()
	assert.Nil(suite.T(), err)
	testutils.AddHandler(testutils.Contains("/api-version"), nil)

	req, _ := http.NewRequest(http.MethodGet, "/api-version", nil)

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), http.StatusInternalServerError, response.StatusCode)
}

func (suite *TestSuite) TestController_HandleGetAllDatabasesByNamespace() {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})

	testRespBody, _ := json.Marshal([]map[string]interface{}{{"connectionProperties": "some-connections"}})
	testutils.AddHandler(testutils.Contains("/list"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})

	app, err := fiberserver.New().Process()
	testToken := testutils.GetSignedTokenWithClaims("test", []string{"ROLE_TEST_ROLE"}, map[string]interface{}{"owner": "test-service"})
	assert.Nil(suite.T(), err)
	app.Get("/list", suite.controller.HandleGettingAllDatabasesByNamespaceV3)

	req, _ := http.NewRequest(http.MethodGet, "/list", nil)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	time.Sleep(5 * time.Second)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGetAllDatabasesByNamespaceV3_ForwardingError() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Get("/list", suite.controller.HandleGettingAllDatabasesByNamespaceV3)
	testutils.AddHandler(testutils.Contains("/list"), nil)

	req, _ := http.NewRequest(http.MethodGet, "/list", nil)

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": MsgErrorForwardingRequestToDbaas,
	})
	assert.Equal(suite.T(), http.StatusInternalServerError, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGetAllDatabasesByNamespaceV3_NamespaceError() {
	testCreds := service.NewBasicCreds("test-user", []byte("test-pass"))
	forwarder := service.NewForwarder(testutils.GetMockServerUrl(), testCreds, testutils.GetMockServerClient())
	namespace := "test-namespace"
	restClient := GetMockRestClient(serialize(client.CompositeStructure{Baseline: "ns-1", Satellites: []string{"ns-2"}}), http.StatusOK, nil)
	controlPlaneClient := client.NewControlPlaneClient(restClient)
	securityService := service.NewSecurityService("", true, namespace, controlPlaneClient, nil)
	controller := NewController(securityService, forwarder)

	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Get("/list", controller.HandleGettingAllDatabasesByNamespaceV3)

	req, _ := http.NewRequest(http.MethodGet, "/list", nil)

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": MsgNotAllowedToAccessNamespace,
	})
	assert.Equal(suite.T(), http.StatusForbidden, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_RequestMustContainClassifier() {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	testRespBody, _ := json.Marshal(map[string]interface{}{
		"namespace": "test-namespace",
	})

	testutils.AddHandler(testutils.Contains("/databases"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})

	app, err := fiberserver.New().Process()
	testToken := testutils.GetSignedTokenWithClaims("test", []string{"ROLE_TEST_ROLE"}, map[string]interface{}{"clientId": "test-service"})
	assert.Nil(suite.T(), err)
	app.Put("/databases", suite.controller.HandleGetOrCreateDatabaseV3)
	req, _ := http.NewRequest(http.MethodPut, "/databases", bytes.NewBuffer(testRespBody))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	time.Sleep(5 * time.Second)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), http.StatusBadRequest, response.StatusCode)

	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), `{"error":"request must contain a classifier"}`, string(respBody))
}

func (suite *TestSuite) TestController_RequestMustContainNamespaceInClassifier() {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	testRespBody, _ := json.Marshal(map[string]interface{}{
		"namespace":  "test-namespace",
		"classifier": map[string]interface{}{"microserviceName": "test-ms"},
	})
	testutils.AddHandler(testutils.Contains("/databases"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})

	app, err := fiberserver.New().Process()
	testToken := testutils.GetSignedTokenWithClaims("test", []string{"ROLE_TEST_ROLE"}, map[string]interface{}{"clientId": "test-service"})
	assert.Nil(suite.T(), err)
	app.Put("/databases", suite.controller.HandleGetOrCreateDatabaseV3)
	req, _ := http.NewRequest(http.MethodPut, "/databases", bytes.NewBuffer(testRespBody))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	time.Sleep(5 * time.Second)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), http.StatusBadRequest, response.StatusCode)

	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), `{"error":"request must contain namespace in a classifier"}`, string(respBody))
}

func (suite *TestSuite) TestGetNamespaceFromClassifier_NoClassifierOrNamespace() {
	body := map[string]interface{}{"namespace": "ns-1"}

	namespace, err := GetNamespaceFromClassifier(body["classifier"])

	suite.ErrorContains(err, "request must contain a classifier")
	suite.Empty(namespace)

	// second case

	body = map[string]interface{}{
		"namespace":  "ns-1",
		"classifier": map[string]interface{}{"serviceName": "ms-name", "microserviceName": "test-service"},
	}

	namespace, err = GetNamespaceFromClassifier(body["classifier"])
	suite.ErrorContains(err, "request must contain namespace in a classifier")
	suite.Empty(namespace)
}

func (suite *TestSuite) TestGetNamespaceFromClassifier_MustBeSuccess() {
	body := map[string]interface{}{
		"namespace":  "ns-1",
		"classifier": map[string]interface{}{"serviceName": "ms-name", "scope": "service", "namespace": "ns-1"},
	}

	namespace, err := GetNamespaceFromClassifier(body["classifier"])
	suite.Nil(err)
	suite.Equal("ns-1", namespace)
}

func (suite *TestSuite) TestController_HandleGetOrCreateDatabaseV3_EmptyBody() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Put("/databases", suite.controller.HandleGetOrCreateDatabaseV3)

	req, _ := http.NewRequest(http.MethodPut, "/databases", nil)

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": "request body in create database handler is nil",
	})
	assert.Equal(suite.T(), http.StatusBadRequest, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGetOrCreateDatabaseV3_WrongBodyContentType() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Put("/databases", suite.controller.HandleGetOrCreateDatabaseV3)

	badBody := []byte("bad_body_non_xml_content")
	req, _ := http.NewRequest(http.MethodPut, "/databases", bytes.NewBuffer(badBody))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationXML)

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": "Failed to read request body in create database handler",
	})
	assert.Equal(suite.T(), http.StatusInternalServerError, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGetOrCreateDatabaseV3_WrongTenant() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Put("/databases", suite.controller.HandleGetOrCreateDatabaseV3)

	body, _ := json.Marshal(map[string]interface{}{
		"classifier": map[string]interface{}{"namespace": "test-namespace", "tenantId": "classifier_tenant", "microserviceName": "test-service"},
	})
	req, _ := http.NewRequest(http.MethodPut, "/databases", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(TenantHeader, "header_tenant")

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": MsgNotAllowedToAccessTenant,
	})
	assert.Equal(suite.T(), http.StatusForbidden, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGetOrCreateDatabaseV3_NoOwner() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Put("/databases", suite.controller.HandleGetOrCreateDatabaseV3)

	body, _ := json.Marshal(map[string]interface{}{"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"}})
	testToken := testutils.GetSignedTokenWithClaims("test", nil, nil)
	testRespBody, _ := json.Marshal(map[string]interface{}{
		"namespace":  "test-namespace",
		"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"},
	})
	testutils.AddHandler(testutils.Contains("/databases"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})

	req, _ := http.NewRequest(http.MethodPut, "/databases", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGetOrCreateDatabaseV3_ForwardingError() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Put("/databases", suite.controller.HandleGetOrCreateDatabaseV3)
	testutils.AddHandler(testutils.Contains("/databases"), nil)

	body, _ := json.Marshal(map[string]interface{}{"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"}})
	testToken := testutils.GetSignedTokenWithClaims("test", nil, map[string]interface{}{"owner": "test-service"})
	req, _ := http.NewRequest(http.MethodPut, "/databases", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": MsgErrorForwardingRequestToDbaas,
	})
	assert.Equal(suite.T(), http.StatusInternalServerError, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGetOrCreateDatabaseV3_NamespaceError() {
	testCreds := service.NewBasicCreds("test-user", []byte("test-pass"))
	forwarder := service.NewForwarder(testutils.GetMockServerUrl(), testCreds, testutils.GetMockServerClient())
	namespace := "test-namespace"
	restClient := GetMockRestClient(serialize(client.CompositeStructure{Baseline: "ns-1", Satellites: []string{"ns-2"}}), http.StatusOK, nil)
	controlPlaneClient := client.NewControlPlaneClient(restClient)
	securityService := service.NewSecurityService("", true, namespace, controlPlaneClient, nil)
	controller := NewController(securityService, forwarder)

	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Put("/databases", controller.HandleGetOrCreateDatabaseV3)

	body, _ := json.Marshal(map[string]interface{}{"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"}})
	req, _ := http.NewRequest(http.MethodPut, "/databases", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": MsgNotAllowedToAccessNamespace,
	})
	assert.Equal(suite.T(), http.StatusForbidden, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGettingConnectionByClassifierV3_WrongTenant() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Post("/get-by-classifier", suite.controller.HandleGettingConnectionByClassifierV3)

	body, _ := json.Marshal(map[string]interface{}{
		"classifier": map[string]interface{}{"namespace": "test-namespace", "tenantId": "classifier_tenant", "microserviceName": "test-service"},
	})
	req, _ := http.NewRequest(http.MethodPost, "/get-by-classifier", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(TenantHeader, "header_tenant")

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": MsgNotAllowedToAccessTenant,
	})
	assert.Equal(suite.T(), http.StatusForbidden, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGettingConnectionByClassifierV3_NoNamespaceInClassifier() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Post("/get-by-classifier", suite.controller.HandleGettingConnectionByClassifierV3)

	body, _ := json.Marshal(map[string]interface{}{
		"classifier": map[string]interface{}{"microserviceName": "test-service"},
	})
	req, _ := http.NewRequest(http.MethodPost, "/get-by-classifier", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": "request must contain namespace in a classifier",
	})
	assert.Equal(suite.T(), http.StatusBadRequest, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGettingConnectionByClassifierV3_NoOwner() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Post("/get-by-classifier", suite.controller.HandleGettingConnectionByClassifierV3)

	body, _ := json.Marshal(map[string]interface{}{
		"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"},
	})
	testToken := testutils.GetSignedTokenWithClaims("test", nil, nil)
	testRespBody, _ := json.Marshal(map[string]interface{}{
		"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"},
	})
	testutils.AddHandler(testutils.Contains("/get-by-classifier"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})

	req, _ := http.NewRequest(http.MethodPost, "/get-by-classifier", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGettingConnectionByClassifierV3_ForwardingError() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Post("/get-by-classifier", suite.controller.HandleGettingConnectionByClassifierV3)
	testutils.AddHandler(testutils.Contains("/get-by-classifier"), nil)

	body, _ := json.Marshal(map[string]interface{}{
		"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"},
	})
	testToken := testutils.GetSignedTokenWithClaims("test", nil, map[string]interface{}{"owner": "test-service"})
	req, _ := http.NewRequest(http.MethodPost, "/get-by-classifier", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": MsgErrorForwardingRequestToDbaas,
	})
	assert.Equal(suite.T(), http.StatusInternalServerError, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGettingConnectionByClassifierV3_NamespaceError() {
	testCreds := service.NewBasicCreds("test-user", []byte("test-pass"))
	forwarder := service.NewForwarder(testutils.GetMockServerUrl(), testCreds, testutils.GetMockServerClient())
	namespace := "test-namespace"
	restClient := GetMockRestClient(serialize(client.CompositeStructure{Baseline: "ns-1", Satellites: []string{"ns-2"}}), http.StatusOK, nil)
	controlPlaneClient := client.NewControlPlaneClient(restClient)
	securityService := service.NewSecurityService("", true, namespace, controlPlaneClient, nil)
	controller := NewController(securityService, forwarder)

	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Post("/get-by-classifier", controller.HandleGettingConnectionByClassifierV3)

	body, _ := json.Marshal(map[string]interface{}{"classifier": map[string]interface{}{"namespace": "test-namespace", "microserviceName": "test-service"}})
	//testToken := testutils.GetSignedTokenWithClaims("test", nil, map[string]interface{}{"owner": "test-service"})
	req, _ := http.NewRequest(http.MethodPost, "/get-by-classifier", bytes.NewBuffer(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	//req.Header.Set(AuthorizationContextName, testutils.AuthHeaderValue(testToken))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": MsgNotAllowedToAccessNamespace,
	})
	assert.Equal(suite.T(), http.StatusForbidden, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGettingPhysicalDatabases() {
	testRespBody, _ := json.Marshal(map[string]interface{}{
		"key": "value",
	})
	testutils.AddHandler(testutils.Contains("/list"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})

	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Get("/list", suite.controller.HandleGettingPhysicalDatabases)

	req, _ := http.NewRequest(http.MethodGet, "/list", bytes.NewBuffer(testRespBody))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_HandleGettingPhysicalDatabases_ForwardingError() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	testutils.AddHandler(testutils.Contains("/list"), nil)

	body, _ := json.Marshal(map[string]interface{}{
		"key": "value",
	})
	app.Get("/list", suite.controller.HandleGettingPhysicalDatabases)

	req, _ := http.NewRequest(http.MethodGet, "/list", bytes.NewBuffer(body))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": MsgErrorForwardingRequestToDbaas,
	})
	assert.Equal(suite.T(), http.StatusInternalServerError, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_loadConfigParameter() {
	confValue := "value"
	fileName := "fileName"
	envName := "testConf"
	err := os.WriteFile(fileName, []byte(confValue), os.ModePerm)
	assert.Equal(suite.T(), nil, err)

	parameter := LoadConfigParameter(fileName, envName)
	assert.Equal(suite.T(), confValue, parameter)
	defer os.Remove(fileName)
}

func (suite *TestSuite) TestController_loadConfigParameter_WrongFileName() {
	envName := "env.name"
	envValue := "envValue"
	err := os.Setenv(envName, envValue)
	assert.Equal(suite.T(), nil, err)
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	parameter := LoadConfigParameter("notExistingFileName", envName)
	assert.Equal(suite.T(), envValue, parameter)
	defer os.Unsetenv(envName)
}

func (suite *TestSuite) TestControllerUtils_getTokenFromRequest_NoAuthToken() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Get("/list", suite.handleGetToken)

	req, _ := http.NewRequest(http.MethodGet, "/list", nil)
	response, err := app.Test(req, -1)

	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), []byte("controller: unauthorized request"), respBody)
}

func (suite *TestSuite) handleGetToken(ctx *fiber.Ctx) error {
	token, err := suite.controller.GetTokenFromRequest(context.Background(), ctx)
	if err != nil {
		return RespondWithBytes(ctx, 0, []byte(err.Error()))
	} else {
		marshal, err := json.Marshal(token)
		assert.Nil(suite.T(), err)
		return RespondWithBytes(ctx, 0, marshal)
	}
}

func (suite *TestSuite) TestController_ApplyConfig() {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})

	testRespBody, _ := json.Marshal([]map[string]interface{}{{"status": "COMPLETED"}})
	testutils.AddHandler(testutils.Contains("/api/declarations/v1/apply"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(testRespBody)
	})

	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Post("/api/declarations/v1/apply", suite.controller.ForwardHandler)

	testReqBody, _ := json.Marshal(map[string]interface{}{"kind": "DBaaS"})
	req, _ := http.NewRequest(http.MethodPost, "/api/declarations/v1/apply", bytes.NewBuffer(testReqBody))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	time.Sleep(5 * time.Second)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_GetOperationStatus() {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})

	testRespBody, _ := json.Marshal(map[string]interface{}{"status": "IN_PROGRESS"})
	testutils.AddHandler(testutils.Contains("/operation/1234/status"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusAccepted)
		_, _ = rw.Write(testRespBody)
	})

	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Get("/api/declarations/v1/operation/:trackingId/status", suite.controller.ForwardHandler)
	req, _ := http.NewRequest(http.MethodGet, "/api/declarations/v1/operation/1234/status", nil)

	time.Sleep(5 * time.Second)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), http.StatusAccepted, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_Terminate() {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})

	testutils.AddHandler(testutils.Contains("/operation/1234/terminate"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Post("/api/declarations/v1/operation/:trackingId/terminate", suite.controller.ForwardHandler)
	req, _ := http.NewRequest(http.MethodPost, "/api/declarations/v1/operation/1234/terminate", nil)

	time.Sleep(5 * time.Second)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
}

func (suite *TestSuite) TestController_ApplyConfig_ForwardingError() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	testutils.AddHandler(testutils.Contains("/api/declarations/v1/apply"), nil)

	app.Post("/api/declarations/v1/apply", suite.controller.ForwardHandler)

	testReqBody, _ := json.Marshal(map[string]interface{}{"kind": "DBaaS"})
	req, _ := http.NewRequest(http.MethodPost, "/api/declarations/v1/apply", bytes.NewBuffer(testReqBody))

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": MsgErrorForwardingRequestToDbaas,
	})
	assert.Equal(suite.T(), http.StatusInternalServerError, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_GetOperationStatus_ForwardingError() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	testutils.AddHandler(testutils.Contains("/operation/1234/status"), nil)

	app.Get("/api/declarations/v1/operation/:trackingId/status", suite.controller.ForwardHandler)

	req, _ := http.NewRequest(http.MethodGet, "/api/declarations/v1/operation/1234/status", nil)

	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	testRespBody, _ := json.Marshal(map[string]interface{}{
		"error": MsgErrorForwardingRequestToDbaas,
	})
	assert.Equal(suite.T(), http.StatusInternalServerError, response.StatusCode)
	assert.Equal(suite.T(), testRespBody, respBody)
}

func (suite *TestSuite) TestController_PostComposite() {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	testReqBody, _ := json.Marshal(map[string]interface{}{"id": "ns-1", "namespaces": []string{"ns-2", "ns-3"}})
	testutils.AddHandler(testutils.Contains("/api/composite/v1/structures"), func(rw http.ResponseWriter, req *http.Request) {
		body, er := io.ReadAll(req.Body)
		assert.Equal(suite.T(), testReqBody, body)
		assert.Nil(suite.T(), er)
		rw.WriteHeader(http.StatusNoContent)
	})

	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Post("/api/composite/v1/structures", suite.controller.ForwardHandler)

	req, _ := http.NewRequest(http.MethodPost, "/api/composite/v1/structures", bytes.NewBuffer(testReqBody))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	time.Sleep(5 * time.Second)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), http.StatusNoContent, response.StatusCode)
}

func GetMockRestClient(respBody string, httpCode int, err error) *client.RestClient {
	restClient := client.NewRestClient("")
	restClient.GetToken = func(ctx context.Context) (string, error) {
		return "", nil
	}
	restClient.Do = func(request *fasthttp.Request, response *fasthttp.Response) error {
		response.SetBodyString(respBody)
		response.SetStatusCode(httpCode)
		return err
	}
	return restClient
}

func serialize(obj interface{}) string {
	bytes, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}
