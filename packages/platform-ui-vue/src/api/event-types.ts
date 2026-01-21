import { bffFetch } from './client';

// Types (will be replaced by Hey API generated types)
export type EventTypeStatus = 'CURRENT' | 'ARCHIVE';
export type SchemaType = 'JSON_SCHEMA' | 'PROTO' | 'XSD';
export type SpecVersionStatus = 'FINALISING' | 'CURRENT' | 'DEPRECATED';

export interface SpecVersion {
  version: string;
  mimeType: string;
  schema?: string;
  schemaType: SchemaType;
  status: SpecVersionStatus;
}

export interface EventType {
  id: string;
  code: string;
  application: string;
  subdomain: string;
  aggregate: string;
  event: string;
  name: string;
  description?: string;
  status: EventTypeStatus;
  specVersions: SpecVersion[];
  createdAt: string;
  updatedAt: string;
}

export interface EventTypeListResponse {
  items: EventType[];
  total: number;
}

export interface FilterOptionsResponse {
  options: string[];
}

export interface CreateEventTypeRequest {
  code: string;
  name: string;
  description?: string;
}

export interface UpdateEventTypeRequest {
  name?: string;
  description?: string;
}

export interface AddSchemaRequest {
  version: string;
  mimeType: string;
  schema: string;
  schemaType: SchemaType;
}

export interface EventTypeFilters {
  applications?: string[];
  subdomains?: string[];
  aggregates?: string[];
  status?: EventTypeStatus;
}

// API functions - using BFF endpoints for JavaScript-safe string IDs
export const eventTypesApi = {
  list(filters: EventTypeFilters = {}): Promise<EventTypeListResponse> {
    const params = new URLSearchParams();
    filters.applications?.forEach(a => params.append('application', a));
    filters.subdomains?.forEach(s => params.append('subdomain', s));
    filters.aggregates?.forEach(a => params.append('aggregate', a));
    if (filters.status) params.set('status', filters.status);

    const query = params.toString();
    return bffFetch(`/event-types${query ? `?${query}` : ''}`);
  },

  get(id: string): Promise<EventType> {
    return bffFetch(`/event-types/${id}`);
  },

  create(data: CreateEventTypeRequest): Promise<EventType> {
    return bffFetch('/event-types', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },

  update(id: string, data: UpdateEventTypeRequest): Promise<EventType> {
    return bffFetch(`/event-types/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    });
  },

  delete(id: string): Promise<void> {
    return bffFetch(`/event-types/${id}`, { method: 'DELETE' });
  },

  archive(id: string): Promise<EventType> {
    return bffFetch(`/event-types/${id}/archive`, { method: 'POST' });
  },

  addSchema(id: string, data: AddSchemaRequest): Promise<EventType> {
    return bffFetch(`/event-types/${id}/schemas`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },

  finaliseSchema(id: string, version: string): Promise<EventType> {
    return bffFetch(`/event-types/${id}/schemas/${version}/finalise`, {
      method: 'POST',
    });
  },

  deprecateSchema(id: string, version: string): Promise<EventType> {
    return bffFetch(`/event-types/${id}/schemas/${version}/deprecate`, {
      method: 'POST',
    });
  },

  // Filter options
  getApplications(): Promise<FilterOptionsResponse> {
    return bffFetch('/event-types/filters/applications');
  },

  getSubdomains(applications?: string[]): Promise<FilterOptionsResponse> {
    const params = new URLSearchParams();
    applications?.forEach(a => params.append('application', a));
    const query = params.toString();
    return bffFetch(`/event-types/filters/subdomains${query ? `?${query}` : ''}`);
  },

  getAggregates(applications?: string[], subdomains?: string[]): Promise<FilterOptionsResponse> {
    const params = new URLSearchParams();
    applications?.forEach(a => params.append('application', a));
    subdomains?.forEach(s => params.append('subdomain', s));
    const query = params.toString();
    return bffFetch(`/event-types/filters/aggregates${query ? `?${query}` : ''}`);
  },
};
