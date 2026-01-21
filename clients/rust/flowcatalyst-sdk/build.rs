//! Build script for FlowCatalyst SDK
//! Generates API client from OpenAPI specification using progenitor

use std::fs;
use std::io::Write;
use std::path::Path;

fn main() {
    println!("cargo:rerun-if-changed=../openapi.yaml");
    println!("cargo:rerun-if-changed=build.rs");

    let spec_path = Path::new("../openapi.yaml");

    if !spec_path.exists() {
        panic!(
            "OpenAPI spec not found at {:?}. Run this from the flowcatalyst-sdk directory.",
            spec_path
        );
    }

    // Read the YAML file
    let yaml_content = fs::read_to_string(spec_path).expect("Failed to read OpenAPI spec");

    // Parse YAML to JSON Value first so we can add operation IDs
    let mut spec_value: serde_json::Value =
        serde_yml::from_str(&yaml_content).expect("Failed to parse OpenAPI YAML spec");

    // Add operation IDs to paths that are missing them
    add_operation_ids(&mut spec_value);

    // Now parse as OpenAPI
    let spec: openapiv3::OpenAPI = serde_json::from_value(spec_value)
        .expect("Failed to convert to OpenAPI spec after adding operation IDs");

    // Generate client code using progenitor
    let mut generator = progenitor::Generator::default();

    let tokens = generator
        .generate_tokens(&spec)
        .expect("Failed to generate client code");

    let ast = syn::parse2(tokens).expect("Failed to parse generated tokens");
    let content = prettyplease::unparse(&ast);

    let out_dir = std::env::var("OUT_DIR").unwrap();
    let dest_path = Path::new(&out_dir).join("generated_client.rs");

    let mut out_file = fs::File::create(&dest_path).expect("Failed to create output file");
    out_file
        .write_all(content.as_bytes())
        .expect("Failed to write generated client");

    println!("cargo:rustc-env=GENERATED_CLIENT={}", dest_path.display());
}

/// Add operation IDs to all operations that are missing them
fn add_operation_ids(spec: &mut serde_json::Value) {
    if let Some(paths) = spec.get_mut("paths").and_then(|p| p.as_object_mut()) {
        for (path, path_item) in paths.iter_mut() {
            if let Some(path_obj) = path_item.as_object_mut() {
                for method in &["get", "post", "put", "patch", "delete", "head", "options"] {
                    if let Some(operation) = path_obj.get_mut(*method) {
                        if let Some(op_obj) = operation.as_object_mut() {
                            if !op_obj.contains_key("operationId") {
                                let operation_id = generate_operation_id(path, method);
                                op_obj.insert(
                                    "operationId".to_string(),
                                    serde_json::Value::String(operation_id),
                                );
                            }
                        }
                    }
                }
            }
        }
    }
}

/// Generate an operation ID from path and method
fn generate_operation_id(path: &str, method: &str) -> String {
    // Convert path like "/admin/platform/anchor-domains/{id}" to "admin_platform_anchor_domains_id"
    let path_parts: Vec<&str> = path
        .split('/')
        .filter(|s| !s.is_empty())
        .map(|s| {
            // Remove curly braces from path parameters
            s.trim_start_matches('{').trim_end_matches('}')
        })
        .collect();

    let path_snake = path_parts.join("_").replace('-', "_");

    // Create operation ID like "get_admin_platform_anchor_domains"
    format!("{}_{}", method, path_snake)
}
