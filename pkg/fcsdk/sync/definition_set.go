package sync

// DefinitionSet is one application's bundled definitions. Build it with
// ForApplication + per-category Add* methods, then hand it to
// Synchronizer.Sync.
type DefinitionSet struct {
	ApplicationCode string
	Roles           []RoleDefinition
	EventTypes      []EventTypeDefinition
	Subscriptions   []SubscriptionDefinition
	DispatchPools   []DispatchPoolDefinition
	Principals      []PrincipalDefinition
	Processes       []ProcessDefinition
}

// ForApplication starts a new set scoped to one application code.
func ForApplication(applicationCode string) *DefinitionSet {
	return &DefinitionSet{ApplicationCode: applicationCode}
}

// AddRole appends a role.
func (s *DefinitionSet) AddRole(r RoleDefinition) *DefinitionSet {
	s.Roles = append(s.Roles, r)
	return s
}

// AddEventType appends an event type.
func (s *DefinitionSet) AddEventType(e EventTypeDefinition) *DefinitionSet {
	s.EventTypes = append(s.EventTypes, e)
	return s
}

// AddSubscription appends a subscription.
func (s *DefinitionSet) AddSubscription(sub SubscriptionDefinition) *DefinitionSet {
	s.Subscriptions = append(s.Subscriptions, sub)
	return s
}

// AddDispatchPool appends a dispatch pool.
func (s *DefinitionSet) AddDispatchPool(d DispatchPoolDefinition) *DefinitionSet {
	s.DispatchPools = append(s.DispatchPools, d)
	return s
}

// AddPrincipal appends a principal.
func (s *DefinitionSet) AddPrincipal(p PrincipalDefinition) *DefinitionSet {
	s.Principals = append(s.Principals, p)
	return s
}

// AddProcess appends a process.
func (s *DefinitionSet) AddProcess(p ProcessDefinition) *DefinitionSet {
	s.Processes = append(s.Processes, p)
	return s
}
