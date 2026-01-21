/**
 * Clients Admin API
 *
 * REST endpoints for client management.
 */

import type { FastifyInstance } from 'fastify';
import { Type, type Static } from '@sinclair/typebox';
import {
	sendResult,
	jsonCreated,
	jsonSuccess,
	noContent,
	notFound,
	badRequest,
	safeValidate,
} from '@flowcatalyst/http';
import { Result } from '@flowcatalyst/application';
import type { UseCase } from '@flowcatalyst/application';

import type {
	CreateClientCommand,
	UpdateClientCommand,
	ChangeClientStatusCommand,
	DeleteClientCommand,
	AddClientNoteCommand,
} from '../../application/index.js';
import type {
	ClientCreated,
	ClientUpdated,
	ClientStatusChanged,
	ClientDeleted,
	ClientNoteAdded,
	ClientStatus,
} from '../../domain/index.js';
import type { ClientRepository } from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { CLIENT_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// Request schemas using TypeBox
const CreateClientSchema = Type.Object({
	name: Type.String({ minLength: 1, maxLength: 255 }),
	identifier: Type.String({ minLength: 1, maxLength: 60 }),
});

const UpdateClientSchema = Type.Object({
	name: Type.String({ minLength: 1, maxLength: 255 }),
});

const ChangeStatusSchema = Type.Object({
	reason: Type.Optional(Type.Union([Type.String({ maxLength: 255 }), Type.Null()])),
	note: Type.Optional(Type.Union([Type.String({ maxLength: 1000 }), Type.Null()])),
});

const AddNoteSchema = Type.Object({
	category: Type.String({ minLength: 1, maxLength: 50 }),
	text: Type.String({ minLength: 1, maxLength: 1000 }),
});

type CreateClientBody = Static<typeof CreateClientSchema>;
type UpdateClientBody = Static<typeof UpdateClientSchema>;
type ChangeStatusBody = Static<typeof ChangeStatusSchema>;
type AddNoteBody = Static<typeof AddNoteSchema>;

// Response schemas for client
interface ClientNoteResponse {
	category: string;
	text: string;
	addedBy: string;
	addedAt: string;
}

interface ClientResponse {
	id: string;
	name: string;
	identifier: string;
	status: string;
	statusReason: string | null;
	statusChangedAt: string | null;
	notes: ClientNoteResponse[];
	createdAt: string;
	updatedAt: string;
}

interface ClientsListResponse {
	clients: ClientResponse[];
	total: number;
	page: number;
	pageSize: number;
}

/**
 * Dependencies for the clients API.
 */
export interface ClientsRoutesDeps {
	readonly clientRepository: ClientRepository;
	readonly createClientUseCase: UseCase<CreateClientCommand, ClientCreated>;
	readonly updateClientUseCase: UseCase<UpdateClientCommand, ClientUpdated>;
	readonly changeClientStatusUseCase: UseCase<ChangeClientStatusCommand, ClientStatusChanged>;
	readonly deleteClientUseCase: UseCase<DeleteClientCommand, ClientDeleted>;
	readonly addClientNoteUseCase: UseCase<AddClientNoteCommand, ClientNoteAdded>;
}

/**
 * Register client admin API routes.
 */
export async function registerClientsRoutes(fastify: FastifyInstance, deps: ClientsRoutesDeps): Promise<void> {
	const {
		clientRepository,
		createClientUseCase,
		updateClientUseCase,
		changeClientStatusUseCase,
		deleteClientUseCase,
		addClientNoteUseCase,
	} = deps;

	// POST /api/admin/clients - Create client
	fastify.post(
		'/clients',
		{
			preHandler: requirePermission(CLIENT_PERMISSIONS.CREATE),
		},
		async (request, reply) => {
			const bodyResult = safeValidate(request.body, CreateClientSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as CreateClientBody;
			const ctx = request.executionContext;

			const command: CreateClientCommand = {
				name: body.name,
				identifier: body.identifier,
			};

			const result = await createClientUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				// Fetch the created client for full response
				const client = await clientRepository.findById(result.value.getData().clientId);
				if (client) {
					return jsonCreated(reply, toClientResponse(client));
				}
			}

			return sendResult(reply, result);
		},
	);

	// GET /api/admin/clients - List clients
	fastify.get(
		'/clients',
		{
			preHandler: requirePermission(CLIENT_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const query = request.query as { page?: string; pageSize?: string };
			const page = parseInt(query.page ?? '0', 10);
			const pageSize = Math.min(parseInt(query.pageSize ?? '20', 10), 100);

			const pagedResult = await clientRepository.findPaged(page, pageSize);

			const response: ClientsListResponse = {
				clients: pagedResult.items.map(toClientResponse),
				total: pagedResult.totalItems,
				page: pagedResult.page,
				pageSize: pagedResult.pageSize,
			};

			return jsonSuccess(reply, response);
		},
	);

	// GET /api/admin/clients/:id - Get client by ID
	fastify.get(
		'/clients/:id',
		{
			preHandler: requirePermission(CLIENT_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const client = await clientRepository.findById(id);

			if (!client) {
				return notFound(reply, `Client not found: ${id}`);
			}

			return jsonSuccess(reply, toClientResponse(client));
		},
	);

	// GET /api/admin/clients/by-identifier/:identifier - Get client by identifier
	fastify.get(
		'/clients/by-identifier/:identifier',
		{
			preHandler: requirePermission(CLIENT_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { identifier } = request.params as { identifier: string };
			const client = await clientRepository.findByIdentifier(identifier);

			if (!client) {
				return notFound(reply, `Client not found with identifier: ${identifier}`);
			}

			return jsonSuccess(reply, toClientResponse(client));
		},
	);

	// PUT /api/admin/clients/:id - Update client
	fastify.put(
		'/clients/:id',
		{
			preHandler: requirePermission(CLIENT_PERMISSIONS.UPDATE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body, UpdateClientSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as UpdateClientBody;
			const ctx = request.executionContext;

			const command: UpdateClientCommand = {
				clientId: id,
				name: body.name,
			};

			const result = await updateClientUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const client = await clientRepository.findById(id);
				if (client) {
					return jsonSuccess(reply, toClientResponse(client));
				}
			}

			return sendResult(reply, result);
		},
	);

	// POST /api/admin/clients/:id/activate - Activate client
	fastify.post(
		'/clients/:id/activate',
		{
			preHandler: requirePermission(CLIENT_PERMISSIONS.ACTIVATE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body ?? {}, ChangeStatusSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = (bodyResult.data ?? {}) as ChangeStatusBody;
			const ctx = request.executionContext;

			const command: ChangeClientStatusCommand = {
				clientId: id,
				newStatus: 'ACTIVE' as ClientStatus,
				reason: body.reason ?? null,
				note: body.note ?? null,
			};

			const result = await changeClientStatusUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const client = await clientRepository.findById(id);
				if (client) {
					return jsonSuccess(reply, toClientResponse(client));
				}
			}

			return sendResult(reply, result);
		},
	);

	// POST /api/admin/clients/:id/suspend - Suspend client
	fastify.post(
		'/clients/:id/suspend',
		{
			preHandler: requirePermission(CLIENT_PERMISSIONS.SUSPEND),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body ?? {}, ChangeStatusSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = (bodyResult.data ?? {}) as ChangeStatusBody;
			const ctx = request.executionContext;

			const command: ChangeClientStatusCommand = {
				clientId: id,
				newStatus: 'SUSPENDED' as ClientStatus,
				reason: body.reason ?? null,
				note: body.note ?? null,
			};

			const result = await changeClientStatusUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const client = await clientRepository.findById(id);
				if (client) {
					return jsonSuccess(reply, toClientResponse(client));
				}
			}

			return sendResult(reply, result);
		},
	);

	// POST /api/admin/clients/:id/deactivate - Deactivate client
	fastify.post(
		'/clients/:id/deactivate',
		{
			preHandler: requirePermission(CLIENT_PERMISSIONS.DEACTIVATE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body ?? {}, ChangeStatusSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = (bodyResult.data ?? {}) as ChangeStatusBody;
			const ctx = request.executionContext;

			const command: ChangeClientStatusCommand = {
				clientId: id,
				newStatus: 'INACTIVE' as ClientStatus,
				reason: body.reason ?? null,
				note: body.note ?? null,
			};

			const result = await changeClientStatusUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const client = await clientRepository.findById(id);
				if (client) {
					return jsonSuccess(reply, toClientResponse(client));
				}
			}

			return sendResult(reply, result);
		},
	);

	// POST /api/admin/clients/:id/notes - Add note to client
	fastify.post(
		'/clients/:id/notes',
		{
			preHandler: requirePermission(CLIENT_PERMISSIONS.UPDATE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body, AddNoteSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as AddNoteBody;
			const ctx = request.executionContext;

			const command: AddClientNoteCommand = {
				clientId: id,
				category: body.category,
				text: body.text,
			};

			const result = await addClientNoteUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const client = await clientRepository.findById(id);
				if (client) {
					return jsonSuccess(reply, toClientResponse(client));
				}
			}

			return sendResult(reply, result);
		},
	);

	// DELETE /api/admin/clients/:id - Delete client
	fastify.delete(
		'/clients/:id',
		{
			preHandler: requirePermission(CLIENT_PERMISSIONS.DELETE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const ctx = request.executionContext;

			const command: DeleteClientCommand = {
				clientId: id,
			};

			const result = await deleteClientUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				return noContent(reply);
			}

			return sendResult(reply, result);
		},
	);
}

/**
 * Convert a Client entity to a ClientResponse.
 */
function toClientResponse(client: {
	id: string;
	name: string;
	identifier: string;
	status: string;
	statusReason: string | null;
	statusChangedAt: Date | null;
	notes: readonly { category: string; text: string; addedBy: string; addedAt: Date }[];
	createdAt: Date;
	updatedAt: Date;
}): ClientResponse {
	return {
		id: client.id,
		name: client.name,
		identifier: client.identifier,
		status: client.status,
		statusReason: client.statusReason,
		statusChangedAt: client.statusChangedAt?.toISOString() ?? null,
		notes: client.notes.map((n) => ({
			category: n.category,
			text: n.text,
			addedBy: n.addedBy,
			addedAt: n.addedAt.toISOString(),
		})),
		createdAt: client.createdAt.toISOString(),
		updatedAt: client.updatedAt.toISOString(),
	};
}
