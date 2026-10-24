/**
 * The ingestor runtime: it asks the platform which ingestions are live, runs each as an Apache Camel
 * route that turns inbound data into records, creates their dispatch jobs through the platform's
 * runtime API, and reports route states, exchange counts, failures and test answers back.
 */
@NullMarked
package io.flowcatalyst.ingestor.runtime;

import org.jspecify.annotations.NullMarked;
