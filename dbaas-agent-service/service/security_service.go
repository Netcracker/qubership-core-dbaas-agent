package service

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/golang-jwt/jwt"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/client"
	"github.com/netcracker/qubership-core-lib-go/v3/security"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
)

type SecurityService struct {
	allowMissingDbOwnerRoles  bool
	namespaceIsolationEnabled bool
	namespace                 string
	controlPlaneClient        *client.ControlPlaneClient
	tenantManagerClient       *client.TenantManagerClient
}

func NewSecurityService(dbaasSecPolicy string, namespaceIsolationEnabled bool,
	namespace string, controlPlaneClient *client.ControlPlaneClient,
	tenantManagerClient *client.TenantManagerClient) *SecurityService {
	if namespaceIsolationEnabled {
		logger.Info("Enabled namespace isolation. dbaas-agent namespace: %v", namespace)
		if namespace == "" {
			log.Fatalf("Failed to resolve namespace of dbaas-agent! NAMESPACE environment variable = %s. The value must not be empty.", namespace)
		}
	} else {
		logger.Infof("Namespace isolation disabled.")
	}
	return &SecurityService{
		allowMissingDbOwnerRoles:  dbaasSecPolicy == "allow",
		namespaceIsolationEnabled: namespaceIsolationEnabled,
		namespace:                 namespace,
		controlPlaneClient:        controlPlaneClient,
		tenantManagerClient:       tenantManagerClient,
	}
}

var ErrForbiddenNamespace = errors.New("controller: access to namespace is forbidden")
var ErrTenantMismatch = errors.New("controller: tenantIds in classifier and header don't match")

func (srv *SecurityService) ValidateToken(ctx context.Context, accessToken string) (*jwt.Token, error) {
	tokenProvider := serviceloader.MustLoad[security.TokenProvider]()
	return tokenProvider.ValidateToken(ctx, accessToken)
}

func (srv *SecurityService) CheckNamespaceIsolation(ctx context.Context, namespaceFromPath string) error {
	if srv.namespaceIsolationEnabled && namespaceFromPath != srv.namespace {
		logger.InfoC(ctx, "Request tries to access namespace %v, but dbaas-agent is placed in %v namespace. Try to get composite structure",
			namespaceFromPath, srv.namespace)
		compositeStructure, err := srv.controlPlaneClient.GetCompositeStructure(ctx)
		if err != nil {
			compositeStructure = srv.controlPlaneClient.GetCompositeStructureFromCache()
			logger.WarnC(ctx, "failed to get composite structure from control-plane: %s. Use cache: %+v", err, compositeStructure)
		}
		if compositeStructure == nil {
			return errors.New("DbaaS-agent namespace and namespace from path is different. " +
				"It's allowed only for composite platform. " +
				"Could not get composite structure from control-plane: " + err.Error())
		}
		if !containsInCompositePlatform(compositeStructure, namespaceFromPath) {
			return errors.New("forbidden! DbaaS-agent namespace and namespace from path must be the same. " +
				"It's prohibited to create or get database from another namespace except composite platform")
		}
	}
	return nil
}

func (srv *SecurityService) CheckNamespaceFromClassifier(ctx context.Context, namespaceFromClassifier string) error {
	if srv.namespaceIsolationEnabled && namespaceFromClassifier != srv.namespace {
		logger.InfoC(ctx, "dbaas-agent namespace and namespace in classifier is different. Try to get composite structure")
		compositeStructure, err := srv.controlPlaneClient.GetCompositeStructure(ctx)
		if err != nil {
			compositeStructure = srv.controlPlaneClient.GetCompositeStructureFromCache()
			logger.WarnC(ctx, "failed to get composite structure from control-plane: %s. Use cache: %+v", err, compositeStructure)
		}
		if compositeStructure == nil {
			return errors.New("DbaaS-agent namespace and namespace in classifier is different. " +
				"It's allowed only for composite platform. " +
				"Could not get composite structure from control-plane: " + err.Error())
		}
		if !containsInCompositePlatform(compositeStructure, namespaceFromClassifier) {
			return errors.New("forbidden! DbaaS-agent namespace and namespace in classifier must be the same. " +
				"It's prohibited to create or get database from another namespace except composite platform")
		}
	}
	return nil
}

func containsInCompositePlatform(compositeStructure *client.CompositeStructure, namespace string) bool {
	if compositeStructure.Baseline == namespace {
		return true
	}
	for _, satellite := range compositeStructure.Satellites {
		if namespace == satellite {
			return true
		}
	}
	return false
}

func (srv *SecurityService) CheckTenantId(ctx context.Context, body map[string]interface{}, tenantFromContext string) error {
	if body == nil {
		return nil
	}
	tenantFromBody := ""
	classifier, found := body["classifier"]
	if found {
		if tenantFromClassifier, found := classifier.(map[string]interface{})["tenantId"]; found {
			tenantFromBody = tenantFromClassifier.(string)
		}
	} else {
		if tenantId, found := body["tenantId"]; found {
			tenantFromBody = tenantId.(string)
		}
	}

	if len(tenantFromBody) > 0 && tenantFromBody != tenantFromContext {
		logger.ErrorC(ctx, "Identifiers are not equal: in secured context: %v , in request body: %v",
			tenantFromContext, tenantFromBody)
		return ErrTenantMismatch
	}

	if len(tenantFromBody) > 0 {
		err := srv.CheckTenantIdExist(ctx, tenantFromBody)
		if err != nil {
			return err
		}
	}
	return nil
}

func (srv *SecurityService) CheckTenantIdExist(ctx context.Context, tenantId string) error {
	logger.InfoC(ctx, "Checking that tenantId is exist. Try to get tenants list from cache")
	tenantList := srv.tenantManagerClient.GetTenantsFromCache()
	if tenantList == nil || !containsInTenantListPlatform(tenantList, tenantId) {
		logger.InfoC(ctx, "TenantId is not founded in cache. Try to get tenants list from tenant-manager")
		tenantListFromTM, err := srv.tenantManagerClient.GetTenantsList(ctx)
		if err != nil {
			return errors.New("Could not get tenants list from tenant-manager: " + err.Error())
		}
		if !containsInTenantListPlatform(tenantListFromTM, tenantId) {
			return errors.New("forbidden! TenantId is not found in tenants list from tenant-manager" +
				"It's prohibited to create or get database with not existing tenantId")
		}
	}
	return nil
}

func containsInTenantListPlatform(tenantList []client.Tenant, tenantId string) bool {
	for _, tenant := range tenantList {
		if tenantId == tenant.TenantId {
			return true
		}
	}

	return false
}

// CheckAnyRoleMatched returns true if at least one required role is found in slice of roles from token.
// Argument 'requiredRoles' must not be null or empty.
func CheckAnyRoleMatched(requiredRoles []string, rolesFromToken []string) bool {
	requiredRoles = withRolePrefix(requiredRoles)
	if len(rolesFromToken) == 0 {
		return false
	}
	for _, requiredRole := range requiredRoles {
		for _, actualRole := range rolesFromToken {
			if requiredRole == actualRole {
				return true
			}
		}
	}
	return false
}

func withRolePrefix(roles []string) []string {
	var updatedRoles []string

	for _, role := range roles {
		if !strings.HasPrefix(role, "ROLE_") {
			updatedRoles = append(updatedRoles, "ROLE_"+role)
		} else {
			updatedRoles = append(updatedRoles, role)
		}
	}

	return updatedRoles
}
