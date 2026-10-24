package io.flowcatalyst.ingestor.contract;

import com.fasterxml.jackson.annotation.JsonInclude;
import org.jspecify.annotations.Nullable;

/**
 * The outcome of the record at {@code index} of a jobs request: the job it is (new, or the one an
 * earlier request made under the same external id), or why it was not made. A record that failed
 * does not fail the others.
 *
 * @param index the record's position in the request
 * @param id the job's id, when it was made
 * @param error why it was not made
 * @param existing true when its external id had already made the job
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public record JobResult(int index, @Nullable String id, @Nullable String error, boolean existing) {}
