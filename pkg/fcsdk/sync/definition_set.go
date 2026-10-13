package sync

import "encoding/json"

// DefinitionSet is one application's bundled definitions. Build it with
// ForApplication + per-category Add* methods, then hand it to
// Synchronizer.Sync.
type DefinitionSet struct {
	ApplicationCode string
	// ClientID scopes the set's connections and subscriptions to one client:
	// its identifier slug (preferred: ids differ per environment) or its id.
	// Empty is the global scope. Scheduled jobs carry their own ClientID.
	ClientID      string
	Roles         []RoleDefinition
	EventTypes    []EventTypeDefinition
	DispatchPools []DispatchPoolDefinition
	Connections   []ConnectionDefinition
	Subscriptions []SubscriptionDefinition
	Processes     []ProcessDefinition
	ScheduledJobs []ScheduledJobDefinition
	Principals    []PrincipalDefinition
	// OpenAPISpec is the application's OpenAPI 3.x / Swagger 2.x document;
	// nil skips the category. Each sync publishes it as the current version.
	OpenAPISpec json.RawMessage
	// Docs is the application's documentation set (full replace).
	Docs []DocDefinition
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

// ForClient scopes the set's connections and subscriptions to a client
// (identifier slug preferred).
func (s *DefinitionSet) ForClient(clientID string) *DefinitionSet {
	s.ClientID = clientID
	return s
}

// AddConnection appends a connection.
func (s *DefinitionSet) AddConnection(c ConnectionDefinition) *DefinitionSet {
	s.Connections = append(s.Connections, c)
	return s
}

// AddScheduledJob appends a scheduled job.
func (s *DefinitionSet) AddScheduledJob(j ScheduledJobDefinition) *DefinitionSet {
	s.ScheduledJobs = append(s.ScheduledJobs, j)
	return s
}

// AddDoc appends a documentation page.
func (s *DefinitionSet) AddDoc(d DocDefinition) *DefinitionSet {
	s.Docs = append(s.Docs, d)
	return s
}

// WithOpenAPISpec sets the application's OpenAPI document.
func (s *DefinitionSet) WithOpenAPISpec(spec json.RawMessage) *DefinitionSet {
	s.OpenAPISpec = spec
	return s
}
