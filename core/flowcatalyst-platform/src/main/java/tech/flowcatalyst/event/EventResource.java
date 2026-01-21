package tech.flowcatalyst.event;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import tech.flowcatalyst.event.operations.CreateEvent;

import java.time.Instant;
import java.util.List;

@Path("/api/events")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@Tag(name = "Events", description = "Event store operations")
public class EventResource {

    @Inject
    EventService eventService;

    @POST
    @Operation(summary = "Create a new event", description = "Creates a new event in the event store. If a deduplicationId is provided and an event with that ID already exists, the existing event is returned (idempotent operation). Dispatch jobs are automatically created for matching subscriptions.")
    @APIResponses({
        @APIResponse(
            responseCode = "201",
            description = "Event created successfully",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = CreateEventResponse.class))
        ),
        @APIResponse(
            responseCode = "200",
            description = "Event already exists (idempotent response)",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = CreateEventResponse.class))
        ),
        @APIResponse(responseCode = "400", description = "Invalid request")
    })
    public Response createEvent(CreateEventRequest request) {
        var result = eventService.create(new CreateEvent(
            request.specVersion(),
            request.type(),
            request.source(),
            request.subject(),
            request.time(),
            request.data(),
            request.correlationId(),
            request.causationId(),
            request.deduplicationId(),
            request.messageGroup(),
            request.contextData()
        ));

        CreateEventResponse response = new CreateEventResponse(
            EventResponse.from(result.event()),
            result.dispatchJobs().size()
        );

        // Return 200 if this was an idempotent replay, 201 if newly created
        if (result.isDuplicate()) {
            return Response.ok(response).build();
        }
        return Response.status(201).entity(response).build();
    }

    @POST
    @Path("/batch")
    @Operation(summary = "Create multiple events in batch", description = "Creates multiple events in a single operation. Maximum batch size is 100 events. Dispatch jobs are automatically created for matching subscriptions.")
    @APIResponses({
        @APIResponse(
            responseCode = "201",
            description = "Events created successfully",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = BatchEventResponse.class))
        ),
        @APIResponse(responseCode = "400", description = "Invalid request or batch size exceeds limit")
    })
    public Response createEventBatch(List<CreateEventRequest> requests) {
        if (requests == null || requests.isEmpty()) {
            return Response.status(400)
                .entity(new ErrorResponse("Request body must contain at least one event"))
                .build();
        }
        if (requests.size() > 100) {
            return Response.status(400)
                .entity(new ErrorResponse("Batch size cannot exceed 100 events"))
                .build();
        }

        List<CreateEvent> operations = requests.stream()
            .map(request -> new CreateEvent(
                request.specVersion(),
                request.type(),
                request.source(),
                request.subject(),
                request.time(),
                request.data(),
                request.correlationId(),
                request.causationId(),
                request.deduplicationId(),
                request.messageGroup(),
                request.contextData()
            ))
            .toList();

        var result = eventService.createBatch(operations);

        List<EventResponse> eventResponses = result.events().stream()
            .map(EventResponse::from)
            .toList();

        return Response.status(201).entity(new BatchEventResponse(
            eventResponses,
            eventResponses.size(),
            result.dispatchJobs().size(),
            result.duplicateCount()
        )).build();
    }

    @GET
    @Path("/{id}")
    @Operation(summary = "Get event by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Event found"),
        @APIResponse(responseCode = "404", description = "Event not found")
    })
    public Response getEvent(@PathParam("id") String id) {
        return eventService.findById(id)
            .map(event -> Response.ok(EventResponse.from(event)).build())
            .orElse(Response.status(404)
                .entity(new ErrorResponse("Event not found: " + id))
                .build());
    }

    // ========================================================================
    // DTOs
    // ========================================================================

    public record CreateEventRequest(
        String specVersion,
        String type,
        String source,
        String subject,
        Instant time,
        String data,
        String correlationId,
        String causationId,
        String deduplicationId,
        String messageGroup,
        List<ContextData> contextData
    ) {}

    public record EventResponse(
        String id,
        String specVersion,
        String type,
        String source,
        String subject,
        String time,
        String data,
        String correlationId,
        String causationId,
        String deduplicationId,
        String messageGroup,
        List<ContextDataResponse> contextData
    ) {
        public static EventResponse from(Event event) {
            List<ContextDataResponse> contextDataResponses = null;
            if (event.contextData() != null) {
                contextDataResponses = event.contextData().stream()
                    .map(cd -> new ContextDataResponse(cd.key(), cd.value()))
                    .toList();
            }

            return new EventResponse(
                event.id(),
                event.specVersion(),
                event.type(),
                event.source(),
                event.subject(),
                event.time() != null ? event.time().toString() : null,
                event.data(),
                event.correlationId(),
                event.causationId(),
                event.deduplicationId(),
                event.messageGroup(),
                contextDataResponses
            );
        }
    }

    public record ContextDataResponse(String key, String value) {}

    public record ErrorResponse(String error) {}

    public record CreateEventResponse(
        EventResponse event,
        int dispatchJobCount
    ) {}

    public record BatchEventResponse(
        List<EventResponse> events,
        int count,
        int dispatchJobCount,
        int duplicateCount
    ) {}
}
