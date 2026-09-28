//! Route pattern compiling and matching: literal segments, `{param}`, and a
//! trailing `{rest...}`.

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum Seg {
    Literal(String),
    Param(String),
    Rest(String),
}

/// Splits a "METHOD /path" registration pattern (same syntax the Go SDK and
/// Go 1.22's http.ServeMux use) into (method, compiled path segments,
/// original path string).
pub(crate) fn compile_pattern(pattern: &str) -> (String, Vec<Seg>, String) {
    let (method, path) = pattern
        .split_once(' ')
        .unwrap_or_else(|| panic!("fn: endpoint pattern must be \"METHOD /path\", got {pattern:?}"));
    let method = method.to_string();
    let path = path.trim();
    if method.is_empty() || path.is_empty() {
        panic!("fn: endpoint pattern must be \"METHOD /path\", got {pattern:?}");
    }
    let segs = path
        .trim_start_matches('/')
        .split('/')
        .filter(|s| !s.is_empty())
        .map(compile_segment)
        .collect();
    (method, segs, path.to_string())
}

fn compile_segment(seg: &str) -> Seg {
    if let Some(inner) = seg.strip_prefix('{').and_then(|s| s.strip_suffix("...}")) {
        Seg::Rest(inner.to_string())
    } else if let Some(inner) = seg.strip_prefix('{').and_then(|s| s.strip_suffix('}')) {
        Seg::Param(inner.to_string())
    } else {
        Seg::Literal(seg.to_string())
    }
}

/// Matches `path_segments` against `pattern`, returning extracted path
/// params on success. A `{rest...}` segment must be last and consumes every
/// remaining segment (including zero).
pub(crate) fn match_segments(
    pattern: &[Seg],
    path_segments: &[&str],
) -> Option<std::collections::BTreeMap<String, String>> {
    let mut params = std::collections::BTreeMap::new();
    let mut pi = 0;
    for (i, seg) in pattern.iter().enumerate() {
        match seg {
            Seg::Rest(name) => {
                let rest = path_segments[pi.min(path_segments.len())..].join("/");
                params.insert(name.clone(), rest);
                debug_assert_eq!(i, pattern.len() - 1, "fn: {{rest...}} must be the final segment");
                return Some(params);
            }
            Seg::Literal(lit) => {
                if pi >= path_segments.len() || path_segments[pi] != lit.as_str() {
                    return None;
                }
                pi += 1;
            }
            Seg::Param(name) => {
                if pi >= path_segments.len() {
                    return None;
                }
                params.insert(name.clone(), path_segments[pi].to_string());
                pi += 1;
            }
        }
    }
    if pi == path_segments.len() {
        Some(params)
    } else {
        None
    }
}

pub(crate) fn split_path(path: &str) -> Vec<&str> {
    path.trim_start_matches('/')
        .split('/')
        .filter(|s| !s.is_empty())
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn compiles_method_and_literal_path() {
        let (m, segs, path) = compile_pattern("POST /events/order-created");
        assert_eq!(m, "POST");
        assert_eq!(path, "/events/order-created");
        assert_eq!(
            segs,
            vec![Seg::Literal("events".into()), Seg::Literal("order-created".into())]
        );
    }

    #[test]
    fn compiles_param_and_rest() {
        let (_, segs, _) = compile_pattern("GET /widgets/{id}/files/{rest...}");
        assert_eq!(
            segs,
            vec![
                Seg::Literal("widgets".into()),
                Seg::Param("id".into()),
                Seg::Literal("files".into()),
                Seg::Rest("rest".into()),
            ]
        );
    }

    #[test]
    #[should_panic(expected = "METHOD /path")]
    fn missing_method_panics() {
        compile_pattern("/healthz");
    }

    #[test]
    fn matches_literal() {
        let (_, segs, _) = compile_pattern("GET /healthz");
        assert!(match_segments(&segs, &split_path("/healthz")).is_some());
        assert!(match_segments(&segs, &split_path("/other")).is_none());
    }

    #[test]
    fn matches_param_extracts_value() {
        let (_, segs, _) = compile_pattern("GET /hello/{name}");
        let params = match_segments(&segs, &split_path("/hello/world")).unwrap();
        assert_eq!(params.get("name").map(String::as_str), Some("world"));
    }

    #[test]
    fn param_does_not_match_extra_segments() {
        let (_, segs, _) = compile_pattern("GET /hello/{name}");
        assert!(match_segments(&segs, &split_path("/hello/world/extra")).is_none());
    }

    #[test]
    fn rest_consumes_remainder_including_slashes() {
        let (_, segs, _) = compile_pattern("GET /files/{rest...}");
        let params = match_segments(&segs, &split_path("/files/a/b/c")).unwrap();
        assert_eq!(params.get("rest").map(String::as_str), Some("a/b/c"));
    }

    #[test]
    fn rest_matches_zero_segments() {
        let (_, segs, _) = compile_pattern("GET /files/{rest...}");
        let params = match_segments(&segs, &split_path("/files")).unwrap();
        assert_eq!(params.get("rest").map(String::as_str), Some(""));
    }

    #[test]
    fn multiple_params() {
        let (_, segs, _) = compile_pattern("GET /widgets/{id}/parts/{part}");
        let params = match_segments(&segs, &split_path("/widgets/42/parts/7")).unwrap();
        assert_eq!(params.get("id").map(String::as_str), Some("42"));
        assert_eq!(params.get("part").map(String::as_str), Some("7"));
    }
}
