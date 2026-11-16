package io.flowcatalyst.function;

/// The version of the function API: this package, its `testing` helpers, and what the host gives a
/// function. A function jar declares the major version it was built against in its manifest, as
/// [#MANIFEST_ATTRIBUTE], and the ingestor runtime refuses to load a jar of another major version
/// (`LOAD:API_MAJOR`). A jar that declares none is taken as major 1, because every function deployed
/// before the attribute existed declares none and was built against major 1.
///
/// The major changes only when an existing function would stop compiling or working; additions do
/// not change it. It is the major of the ingestor API's version (`ingestor/API_VERSION`), which this
/// module shares with `ingestor-contract` and `ingestor-extension`.
public final class FunctionApi {

    /// The major version of this API.
    public static final int MAJOR = 1;

    /// The jar manifest attribute that declares the major version a function was built against.
    public static final String MANIFEST_ATTRIBUTE = "Ingestor-Function-Api";

    private FunctionApi() {
    }
}
