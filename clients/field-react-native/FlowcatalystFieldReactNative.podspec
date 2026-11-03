require "json"

package = JSON.parse(File.read(File.join(__dir__, "package.json")))

Pod::Spec.new do |s|
  s.name         = "FlowcatalystFieldReactNative"
  s.version      = package["version"]
  s.summary      = package["description"]
  s.license      = package["license"]
  s.homepage     = "https://flowcatalyst.io"
  s.authors      = "FlowCatalyst"
  s.platforms    = { :ios => "13.0" }
  s.source       = { :path => "." }
  s.source_files = "ios/FcField.{m,swift}", "ios/Der/*.swift"
  s.swift_version = "5.0"
  s.frameworks   = "Security", "LocalAuthentication"
  s.dependency "React-Core"
end
