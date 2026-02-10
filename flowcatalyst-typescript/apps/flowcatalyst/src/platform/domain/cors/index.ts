/**
 * CORS Domain
 */

export {
  type CorsAllowedOrigin,
  type NewCorsAllowedOrigin,
  createCorsAllowedOrigin,
} from './cors-allowed-origin.js';

export {
  type CorsOriginAddedData,
  CorsOriginAdded,
  type CorsOriginDeletedData,
  CorsOriginDeleted,
} from './events.js';
