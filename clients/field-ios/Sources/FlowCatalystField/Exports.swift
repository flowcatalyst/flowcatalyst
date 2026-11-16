// `import FlowCatalystField` is all an app needs: the core is re-exported.
// The core module does not exist when these files are compiled together with the core into one module (the React
// Native pod does that), hence the guard: the same bytes work in both builds.
#if canImport(FlowCatalystFieldCore)
@_exported import FlowCatalystFieldCore
#endif
