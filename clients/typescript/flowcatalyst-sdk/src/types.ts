/**
 * Core types for FlowCatalyst SDK
 */

export interface FlowCatalystConfig {
  baseUrl: string
  apiKey?: string
  timeout?: number
}

export interface EventType {
  id: string
  name: string
  version: string
  schema: Record<string, any>
  createdAt: string
}

export interface Subscription {
  id: string
  eventTypeId: string
  endpoint: string
  status: 'active' | 'paused' | 'failed'
  createdAt: string
}

export interface DispatchJob {
  id: string
  eventId: string
  subscriptionId: string
  status: 'pending' | 'processing' | 'completed' | 'failed'
  attempts: number
  createdAt: string
  completedAt?: string
}

export interface ListResponse<T> {
  items: T[]
}

export interface Client {
  id: string
  name: string
  identifier?: string
  status?: string
}

export interface Application {
  id: string
  name: string
  code?: string
  website: string
  logo?: string // SVG text (optional)
}

export interface ApiResponse<T> {
  data?: T
  error?: string
}
