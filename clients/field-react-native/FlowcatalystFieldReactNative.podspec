require "json"

package = JSON.parse(File.read(File.join(__dir__, "package.json")))

Pod::Spec.new do |s|
  s.name         = "FlowcatalystFieldReactNative"
  s.version      = package["version"]
  s.summary      = package["description"]
  s.license      = package["license"]
  s.homepage     = "https://flowcatalyst.io"
  s.authors      = "FlowCatalyst"
  s.platforms    = { :ios => "15.0" }
  s.source       = { :path => "." }

  # One Swift module, named after the pod: the bridge (FcField.mm, FcFieldHost.swift, ...) and a verbatim copy of the
  # field-ios sources (ios/FieldIos, made by scripts/sync-field-ios.mjs). Another pod can `import
  # FlowcatalystFieldReactNative` to reach field-ios's public API (FieldSession.get(config:), FieldConfig,
  # authorize(_:), ...) and `FieldBridgeConfig`. The Objective-C++ module class is not a public header, so the
  # module map stays plain Swift.
  s.source_files = "ios/*.{mm,swift}", "ios/FieldIos/**/*.swift"
  s.swift_version = "5.9"
  s.frameworks   = "Security", "LocalAuthentication", "CryptoKit", "UIKit"
  s.pod_target_xcconfig = {
    "DEFINES_MODULE" => "YES",
    "CLANG_CXX_LANGUAGE_STANDARD" => "c++20",
  }

  # React-Core, the codegen output (ReactCodegen, which holds FcFieldSpec) and the C++ dependencies the TurboModule needs.
  if respond_to?(:install_modules_dependencies, true)
    install_modules_dependencies(s)
  else
    s.dependency "React-Core"
  end
end
