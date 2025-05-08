package domain

type DbOwnerRolesRequest struct {
	Services []Service `json:"services"`
}

type Service struct {
	MicroserviceName string   `json:"microserviceName"`
	DbOwnerRoles     []string `json:"dbOwnerRoles"`
}

type Database struct {
	Id                   string                 `json:"id"`
	Classifier           map[string]interface{} `json:"classifier"`
	ConnectionProperties map[string]interface{} `json:"connectionProperties"`
	Namespace            string                 `json:"namespace"`
	Type                 string                 `json:"type"`
	Name                 string                 `json:"name"`
	DbOwnerRoles         []string               `json:"dbOwnerRoles"`
}
