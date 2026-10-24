/**
 * What an extension implements to add a kind of source to the ingestor runtime. An extension is a
 * jar in the runtime's extensions directory ({@code INGEST_EXTENSIONS_DIR}), or a module on its
 * class path: it lists its {@link io.flowcatalyst.ingestor.extension.IngestExtension} in {@code
 * META-INF/services}, and the extension returns its kind ({@link
 * io.flowcatalyst.ingestor.contract.ExtensionKind}). The runtime reports the kinds it carries to the
 * platform, and an ingestion whose source is {@code custom} with that type runs on it. The runtime
 * does everything after the endpoint: splitting, dispatch jobs, counts, failures and health. {@link
 * PolledSource} and {@link SeenKeys} are for sources polled with code of their own. A jar declares
 * the extension API it was built against ({@link ExtensionApi}).
 */
@NullMarked
package io.flowcatalyst.ingestor.extension;

import org.jspecify.annotations.NullMarked;
