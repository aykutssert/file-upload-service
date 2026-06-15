package auth

type PrincipalType string

const (
	PrincipalTypeService PrincipalType = "service"
	PrincipalTypeUser    PrincipalType = "user"
)

type Principal struct {
	TenantID    string
	SubjectID   string
	Type        PrincipalType
	Role        string
	Permissions map[string]struct{}
}

func (p Principal) HasPermission(permission string) bool {
	_, ok := p.Permissions[permission]
	return ok
}
