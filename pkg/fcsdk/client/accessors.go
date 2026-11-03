package client

// Resource accessors. Each method returns a value type that holds a
// reference back to the client; methods on the value type issue HTTP
// requests via the embedded *FlowCatalystClient.

// EventTypes returns the event-types resource accessor — /api/event-types/*.
func (c *FlowCatalystClient) EventTypes() *EventTypesResource {
	return &EventTypesResource{c: c}
}

// Subscriptions returns the subscriptions resource accessor — /api/subscriptions/*.
func (c *FlowCatalystClient) Subscriptions() *SubscriptionsResource {
	return &SubscriptionsResource{c: c}
}

// DispatchPools returns the dispatch-pools resource accessor — /api/dispatch-pools/*.
func (c *FlowCatalystClient) DispatchPools() *DispatchPoolsResource {
	return &DispatchPoolsResource{c: c}
}

// Applications returns the applications resource accessor — /api/applications/*.
func (c *FlowCatalystClient) Applications() *ApplicationsResource {
	return &ApplicationsResource{c: c}
}

// Processes returns the processes resource accessor — /api/processes/*.
func (c *FlowCatalystClient) Processes() *ProcessesResource {
	return &ProcessesResource{c: c}
}

// Principals returns the principals resource accessor — /api/principals/*.
func (c *FlowCatalystClient) Principals() *PrincipalsResource {
	return &PrincipalsResource{c: c}
}

// Roles returns the roles resource accessor — /api/roles/*.
func (c *FlowCatalystClient) Roles() *RolesResource {
	return &RolesResource{c: c}
}

// Permissions returns the permissions catalogue accessor — /api/roles/permissions/*.
func (c *FlowCatalystClient) Permissions() *PermissionsResource {
	return &PermissionsResource{c: c}
}

// AuditLogs returns the audit logs accessor — /api/audit-logs/*.
func (c *FlowCatalystClient) AuditLogs() *AuditLogsResource {
	return &AuditLogsResource{c: c}
}

// Clients returns the clients (tenants) accessor — /api/clients/*.
func (c *FlowCatalystClient) Clients() *ClientsResource {
	return &ClientsResource{c: c}
}

// Connections returns the connections accessor — /api/connections/*.
func (c *FlowCatalystClient) Connections() *ConnectionsResource {
	return &ConnectionsResource{c: c}
}

// Me returns the current-user accessor — /api/me/*.
func (c *FlowCatalystClient) Me() *MeResource {
	return &MeResource{c: c}
}

// Router returns the message-router monitoring accessor. Uses
// routerBaseURL (or baseURL as fallback) — distinct host from the
// rest of the API.
func (c *FlowCatalystClient) Router() *RouterResource {
	return &RouterResource{c: c}
}

// ScheduledJobs returns the scheduled-jobs accessor — /api/scheduled-jobs/*.
func (c *FlowCatalystClient) ScheduledJobs() *ScheduledJobsResource {
	return &ScheduledJobsResource{c: c}
}

// OpenAPI returns the openapi sync accessor —
// /api/applications/{appCode}/openapi/*.
func (c *FlowCatalystClient) OpenAPI() *OpenAPIResource {
	return &OpenAPIResource{c: c}
}

// PlatformConfig returns the platform-config accessor — /api/config/*.
func (c *FlowCatalystClient) PlatformConfig() *PlatformConfigResource {
	return &PlatformConfigResource{c: c}
}

// FieldWorkers returns the field-workers accessor — /api/field-workers/*.
func (c *FlowCatalystClient) FieldWorkers() *FieldWorkersResource {
	return &FieldWorkersResource{c: c}
}

// FieldDevices returns the field-devices accessor — /api/field-devices/*.
func (c *FlowCatalystClient) FieldDevices() *FieldDevicesResource {
	return &FieldDevicesResource{c: c}
}

// FieldPolicies returns the field-policies accessor — /api/field-policies/*.
func (c *FlowCatalystClient) FieldPolicies() *FieldPoliciesResource {
	return &FieldPoliciesResource{c: c}
}

// FieldTokens returns the field-tokens accessor — /api/field-tokens/*.
func (c *FlowCatalystClient) FieldTokens() *FieldTokensResource {
	return &FieldTokensResource{c: c}
}
