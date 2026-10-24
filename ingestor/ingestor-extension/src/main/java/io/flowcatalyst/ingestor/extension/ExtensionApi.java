package io.flowcatalyst.ingestor.extension;

/**
 * The version of the extension API: this package, the contract's {@code ExtensionKind} and {@code
 * SourceKind}, and what the runtime gives an extension. A jar in the extensions directory declares
 * the major version it was built against in its manifest, as {@value #MANIFEST_ATTRIBUTE}, and the
 * runtime refuses to start with a jar of another major version. The major changes only when an
 * existing extension would stop working; additions do not change it.
 */
public final class ExtensionApi {

  /** The major version of this API. */
  public static final int MAJOR = 1;

  /** The jar manifest attribute that declares the major version an extension was built against. */
  public static final String MANIFEST_ATTRIBUTE = "Ingestor-Extension-Api";

  private ExtensionApi() {}
}
