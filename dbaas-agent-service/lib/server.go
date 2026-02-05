package lib

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/client"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/controller"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/docs"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/service"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/apiversion"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/tracing"
	fiberserver "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/server"
	"github.com/netcracker/qubership-core-lib-go-rest-utils/v2/consul-propertysource"
	routeregistration "github.com/netcracker/qubership-core-lib-go-rest-utils/v2/route-registration"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	constants "github.com/netcracker/qubership-core-lib-go/v3/const"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"

	// swagger docs
	_ "github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/docs"
)

var (
	ctx                 = context.WithValue(context.Background(), "requestId", "")
	logger              logging.Logger
	securityService     *service.SecurityService
	apiVersionService   apiversion.ApiVersionService
	forwarder           *service.Forwarder
	apiController       *controller.Controller
	tenantManagerClient *client.TenantManagerClient
)

const (
	namespaceDatabasePath = "/:namespace/databases"
	microserviceNamespace = "microservice.namespace"
)

func init() {
	consulPS := consul.NewLoggingPropertySource()
	configloader.InitWithSourcesArray(append(configloader.BasePropertySources(), consulPS))
	consul.StartWatchingForPropertiesWithRetry(context.Background(), consulPS, func(event interface{}, err error) {
		// your code here if any action on event is required
	})
	logger = logging.GetLogger("server")
}

func initConfiguration() {

	// reading application config
	defaultDbaasUrl := constants.SelectUrl("http://dbaas-aggregator.dbaas:8080", "https://dbaas-aggregator.dbaas:8443")
	dbaasAddr := configloader.GetOrDefaultString("api.dbaas.address", defaultDbaasUrl)
	username := configloader.GetKoanf().MustString("dbaas.cluster.dba.credentials.username")
	password := configloader.GetKoanf().MustString("dbaas.cluster.dba.credentials.password")

	defaultSecPolicy := configloader.GetOrDefaultString("dbaas.default.sec.policy", "allow")
	namespaceIsolationEnv := configloader.GetOrDefaultString("dbaas.agent.namespace.isolation.enabled", "true")
	namespaceIsolationEnabled, err := strconv.ParseBool(namespaceIsolationEnv)
	if err != nil {
		logger.Errorf("Failed to convert env variable DBAAS_AGENT_NAMESPACE_ISOLATION_ENABLED value '%v' to boolean (%v), using default value: 'true'",
			namespaceIsolationEnv, err)
		namespaceIsolationEnabled = true
	}

	httpRequestTimeoutEnv := configloader.GetOrDefaultString("dbaas.request.timeout.sec", "600")
	httpRequestTimeoutSec, err := strconv.Atoi(httpRequestTimeoutEnv)
	if err != nil {
		logger.Errorf("Failed to convert env variable DBAAS_REQUEST_TIMEOUT_SEC value '%v' to int (%v), using default timeout value: 10 minutes",
			httpRequestTimeoutEnv, err)
		httpRequestTimeoutSec = 600
	}

	logger.Info("Initialize application with target dbaas-aggregator address: '%s'", dbaasAddr)

	// initializing services
	namespace := configloader.GetKoanf().MustString(microserviceNamespace)

	restClient := client.NewRestClient(client.GetInternalGatewayUrl())
	tenantManagerClient = client.NewTenantManagerClient(restClient)

	securityService = service.NewSecurityService(
		defaultSecPolicy,
		namespaceIsolationEnabled,
		namespace,
		client.NewControlPlaneClient(restClient),
		tenantManagerClient)

	clusterDbaCreds := service.NewBasicCreds(username, []byte(password))
	forwarder = service.NewForwarder(
		dbaasAddr,
		clusterDbaCreds,
		service.DefaultHttpClient(time.Duration(httpRequestTimeoutSec)*time.Second))
	apiVersionService, err = service.NewApiVersionService(apiversion.ApiVersionConfig{}, forwarder)
	apiController = controller.NewController(securityService, forwarder)
}

//go:generate go run github.com/swaggo/swag/cmd/swag init --generalInfo /controller/dba_controller.go --parseDependency --parseGoList=false --parseDepth 2
func RunServer() {
	initConfiguration()
	app, err := fiberserver.New(fiber.Config{Network: fiber.NetworkTCP, IdleTimeout: 30 * time.Second}).
		WithPprof("6060").
		WithPrometheus("/prometheus").
		WithTracer(tracing.NewZipkinTracer()).
		WithDeprecatedApiSwitchedOff().
		WithApiVersion(apiVersionService).
		WithLogLevelsInfo().
		Process()
	if err != nil {
		logger.Error("Error while create app because: " + err.Error())
		return
	}

	apiV3Dbaas := app.Group("/api/v3/dbaas")
	apiV3Dbaas.Put(namespaceDatabasePath, apiController.HandleGetOrCreateDatabaseV3)
	apiV3Dbaas.Post(namespaceDatabasePath+"/get-by-classifier/:type", apiController.HandleGettingConnectionByClassifierV3)
	apiV3Dbaas.Delete(namespaceDatabasePath+"/:type", apiController.HandleDeletionByClassifier)
	apiV3Dbaas.Get("/:type/physical_databases", apiController.HandleGettingPhysicalDatabases)
	apiV3Dbaas.Put(namespaceDatabasePath+"/registration/externally_manageable", apiController.HandleRegistrationExternallyManageableDBV3)
	apiV3Dbaas.Get(namespaceDatabasePath+"/list", apiController.HandleGettingAllDatabasesByNamespaceV3)

	apiConfigsDbaas := app.Group("/api/declarations/v1")
	apiConfigsDbaas.Post("/apply", apiController.ForwardHandler)
	apiConfigsDbaas.Get("/operation/:trackingId/status", apiController.ForwardHandler)
	apiConfigsDbaas.Post("/operation/:trackingId/terminate", apiController.ForwardHandler)

	apiComposite := app.Group("/api/composite/v1")
	apiComposite.Get("/structures", apiController.ForwardHandler)
	apiComposite.Get("/structures/:compositeId", apiController.ForwardHandler)
	apiComposite.Post("/structures", apiController.ForwardHandler)
	apiComposite.Delete("/structures/:compositeId/delete", apiController.ForwardHandler)

	app.Get("/health", apiController.HandleGetHealth)
	app.Get("/probes/live", apiController.HandleProbes)
	app.Get("/probes/ready", apiController.HandleProbes)

	// swagger
	app.Get("/swagger-ui/swagger.json", func(ctx *fiber.Ctx) error {
		ctx.Set("Content-Type", "application/json")
		return ctx.Status(http.StatusOK).SendString(docs.SwaggerInfo.ReadDoc())
	})
	routeregistration.NewRegistrar().WithRoutes(
		routeregistration.Route{
			From:      "/api/v1/dbaas/records/databases/db-owner-roles",
			To:        "/api/v1/dbaas/records/databases/db-owner-roles",
			RouteType: routeregistration.Private,
		},
	).Register()

	go BackgroundCleanJob()
	server.StartServer(app, "http.server.bind")
}

func BackgroundCleanJob() {
	ticker := time.NewTicker(2 * time.Minute)

	for _ = range ticker.C {
		logger.Debug("Clean cache job")
		tenantManagerClient.CleanCacheJob()
	}
}
