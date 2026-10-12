//! Local pre-flight validation for each sync category. Every function returns
//! `Err(message)` when the category must not be sent; nothing here touches
//! the network.

use super::definitions::{
    ConnectionDefinition, DispatchPoolDefinition, DocDefinition, EventTypeDefinition,
    PrincipalDefinition, ProcessDefinition, RoleDefinition, ScheduledJobDefinition,
    SubscriptionDefinition,
};

/// Distinct values that occur more than once, in first-duplicate order.
fn duplicates<'a>(keys: impl IntoIterator<Item = &'a str>) -> Vec<String> {
    let mut seen = std::collections::HashSet::new();
    let mut dups: Vec<String> = Vec::new();
    for k in keys {
        if !seen.insert(k) && !dups.iter().any(|d| d == k) {
            dups.push(k.to_string());
        }
    }
    dups
}

fn check_duplicates<'a>(
    kind: &str,
    app: &str,
    keys: impl IntoIterator<Item = &'a str>,
) -> Result<(), String> {
    let dups = duplicates(keys);
    if dups.is_empty() {
        Ok(())
    } else {
        Err(format!(
            "Duplicate {kind} for application \"{app}\": {}",
            dups.join(", ")
        ))
    }
}

/// Short, lowercase role name with any `{app}:` prefix removed — the key the
/// platform stores roles under.
fn role_key(app: &str, name: &str) -> String {
    let lower = name.to_lowercase();
    let prefix = format!("{}:", app.to_lowercase());
    match lower.strip_prefix(&prefix) {
        Some(rest) if !rest.is_empty() => rest.to_string(),
        _ => lower,
    }
}

/// Role wire permissions: lowercased, and each must belong to `app`.
pub(crate) fn roles(app: &str, roles: &[RoleDefinition]) -> Result<(), String> {
    let keys: Vec<String> = roles.iter().map(|r| role_key(app, &r.name)).collect();
    check_duplicates("role names", app, keys.iter().map(String::as_str))?;
    let mut outside: Vec<String> = Vec::new();
    for p in roles.iter().flat_map(|r| r.permissions.iter()) {
        let lower = p.to_lowercase();
        let first = lower.split(':').next().unwrap_or("");
        if first != app && !outside.contains(&lower) {
            outside.push(lower);
        }
    }
    if outside.is_empty() {
        Ok(())
    } else {
        Err(format!(
            "Permissions outside application \"{app}\" (first segment must be the application code): {}",
            outside.join(", ")
        ))
    }
}

pub(crate) fn event_types(app: &str, items: &[EventTypeDefinition]) -> Result<(), String> {
    check_duplicates(
        "event type codes",
        app,
        items.iter().map(|e| e.code.as_str()),
    )?;
    let bad: Vec<&str> = items
        .iter()
        .map(|e| e.code.as_str())
        .filter(|c| {
            let segs: Vec<&str> = c.split(':').collect();
            segs.len() != 4 || segs.iter().any(|s| s.is_empty()) || segs[0] != app
        })
        .collect();
    if bad.is_empty() {
        Ok(())
    } else {
        Err(format!(
            "Invalid event type code(s) for application \"{app}\" (must be {app}:subdomain:aggregate:event, four non-empty segments): {}",
            bad.join(", ")
        ))
    }
}

pub(crate) fn dispatch_pools(app: &str, items: &[DispatchPoolDefinition]) -> Result<(), String> {
    check_duplicates(
        "dispatch pool codes",
        app,
        items.iter().map(|p| p.code.as_str()),
    )
}

pub(crate) fn connections(app: &str, items: &[ConnectionDefinition]) -> Result<(), String> {
    check_duplicates(
        "connection codes",
        app,
        items.iter().map(|c| c.code.as_str()),
    )
}

pub(crate) fn processes(app: &str, items: &[ProcessDefinition]) -> Result<(), String> {
    check_duplicates("process codes", app, items.iter().map(|p| p.code.as_str()))
}

pub(crate) fn principals(app: &str, items: &[PrincipalDefinition]) -> Result<(), String> {
    let keys: Vec<String> = items.iter().map(|p| p.email.to_lowercase()).collect();
    check_duplicates("principal emails", app, keys.iter().map(String::as_str))
}

pub(crate) fn docs(app: &str, items: &[DocDefinition]) -> Result<(), String> {
    check_duplicates("doc slugs", app, items.iter().map(|d| d.slug.as_str()))?;
    let bad: Vec<&str> = items
        .iter()
        .map(|d| d.slug.as_str())
        .filter(|s| !valid_slug(s))
        .collect();
    if bad.is_empty() {
        Ok(())
    } else {
        Err(format!(
            "Invalid doc slug(s) for application \"{app}\" (must match ^[a-z0-9][a-z0-9-]*$): {}",
            bad.join(", ")
        ))
    }
}

/// `^[a-z0-9][a-z0-9-]*$`
fn valid_slug(s: &str) -> bool {
    let mut chars = s.chars();
    match chars.next() {
        Some(c) if c.is_ascii_lowercase() || c.is_ascii_digit() => {}
        _ => return false,
    }
    chars.all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-')
}

/// Scheduled jobs of ONE client group: duplicates by code, and every cron
/// must have 6 whitespace-separated fields (seconds first).
pub(crate) fn scheduled_jobs(
    app: &str,
    scope: &str,
    jobs: &[&ScheduledJobDefinition],
) -> Result<(), String> {
    check_duplicates(
        &format!("scheduled job codes{scope}"),
        app,
        jobs.iter().map(|j| j.code.as_str()),
    )?;
    let mut problems: Vec<String> = Vec::new();
    for j in jobs {
        for cron in &j.crons {
            let n = cron.split_whitespace().count();
            if n != 6 {
                problems.push(format!("{} (cron \"{cron}\" has {n} fields)", j.code));
            }
        }
    }
    if problems.is_empty() {
        Ok(())
    } else {
        Err(format!(
            "Invalid cron for application \"{app}\" (crons are 6 fields, seconds first: sec min hour dom mon dow): {}",
            problems.join(", ")
        ))
    }
}

fn is_http_url(s: &str) -> bool {
    let lower = s.to_ascii_lowercase();
    ["http://", "https://"].iter().any(|p| {
        lower
            .strip_prefix(p)
            .is_some_and(|rest| !rest.is_empty() && !rest.starts_with('/'))
    })
}

/// Resolve a subscription target to an absolute http(s) URL. An absolute
/// target is used as is; a path (`/...`) is joined onto `base`. `None` when
/// the target is empty, not http(s), or a path with no usable base.
pub(crate) fn resolve_target(raw: &str, base: Option<&str>) -> Option<String> {
    let target = raw.trim();
    if target.is_empty() {
        return None;
    }
    if is_http_url(target) {
        return Some(target.to_string());
    }
    if target.starts_with('/') {
        let base = base?.trim().trim_end_matches('/');
        if !is_http_url(base) {
            return None;
        }
        return Some(format!("{base}/{}", target.trim_start_matches('/')));
    }
    None
}

/// Validate and resolve the subscriptions, returning each definition with its
/// resolved absolute target. Fails the whole category on any problem.
pub(crate) fn subscriptions(
    app: &str,
    items: &[SubscriptionDefinition],
    base_url: Option<&str>,
) -> Result<Vec<SubscriptionDefinition>, String> {
    check_duplicates(
        "subscription codes",
        app,
        items.iter().map(|s| s.code.as_str()),
    )?;
    let shared_without_code: Vec<&str> = items
        .iter()
        .filter(|s| {
            s.shared_connection == Some(true)
                && s.connection_code
                    .as_deref()
                    .is_none_or(|c| c.trim().is_empty())
        })
        .map(|s| s.code.as_str())
        .collect();
    if !shared_without_code.is_empty() {
        return Err(format!(
            "sharedConnection requires connectionCode for subscription(s) of application \"{app}\": {}",
            shared_without_code.join(", ")
        ));
    }
    let mut unresolved: Vec<&str> = Vec::new();
    let mut out = Vec::with_capacity(items.len());
    for s in items {
        match resolve_target(&s.target, base_url) {
            Some(t) => {
                let mut s = s.clone();
                s.target = t;
                out.push(s);
            }
            None => unresolved.push(s.code.as_str()),
        }
    }
    if !unresolved.is_empty() {
        return Err(format!(
            "No valid delivery target for subscription(s) of application \"{app}\": {}. `target` is required and must be an absolute http(s) URL, or a path starting with '/' resolved against with_subscription_target_base_url",
            unresolved.join(", ")
        ));
    }
    Ok(out)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn duplicate_message_format() {
        let e = connections(
            "orders",
            &[
                ConnectionDefinition::make("a", "A"),
                ConnectionDefinition::make("a", "A2"),
                ConnectionDefinition::make("b", "B"),
                ConnectionDefinition::make("b", "B2"),
            ],
        )
        .unwrap_err();
        assert_eq!(
            e,
            "Duplicate connection codes for application \"orders\": a, b"
        );
    }

    #[test]
    fn role_permissions_must_start_with_app() {
        let ok = RoleDefinition::make("admin").with_permissions(vec!["Orders:a:b:c".into()]);
        assert!(roles("orders", std::slice::from_ref(&ok)).is_ok());
        let bad = RoleDefinition::make("x").with_permissions(vec!["billing:a:b:c".into()]);
        let e = roles("orders", &[ok, bad]).unwrap_err();
        assert!(e.contains("billing:a:b:c"));
    }

    #[test]
    fn role_short_and_prefixed_names_collide() {
        let e = roles(
            "orders",
            &[
                RoleDefinition::make("admin"),
                RoleDefinition::make("orders:Admin"),
            ],
        )
        .unwrap_err();
        assert!(e.starts_with("Duplicate role names"));
    }

    #[test]
    fn event_type_shape() {
        assert!(event_types("orders", &[EventTypeDefinition::make("orders:a:b:c", "x")]).is_ok());
        for bad in [
            "orders:a:b",
            "orders:a::c",
            "billing:a:b:c",
            "orders:a:b:c:d",
        ] {
            let e = event_types("orders", &[EventTypeDefinition::make(bad, "x")]).unwrap_err();
            assert!(e.contains(bad), "{e}");
        }
    }

    #[test]
    fn slug_shape() {
        assert!(valid_slug("intro-2"));
        for bad in ["", "-a", "A", "a_b", "a b"] {
            assert!(!valid_slug(bad), "{bad}");
        }
    }

    #[test]
    fn cron_needs_six_fields() {
        let ok = ScheduledJobDefinition::make("a", "A").add_cron("0 0 * * * *");
        let bad = ScheduledJobDefinition::make("b", "B").add_cron("0 0 * * *");
        assert!(scheduled_jobs("orders", "", &[&ok]).is_ok());
        let e = scheduled_jobs("orders", "", &[&ok, &bad]).unwrap_err();
        assert!(e.contains("b (cron"));
    }

    #[test]
    fn target_resolution() {
        assert_eq!(
            resolve_target("https://x.example/h", None).as_deref(),
            Some("https://x.example/h")
        );
        assert_eq!(
            resolve_target("/hook", Some("https://x.example/")).as_deref(),
            Some("https://x.example/hook")
        );
        assert_eq!(resolve_target("/hook", None), None);
        assert_eq!(resolve_target("ftp://x/h", None), None);
        assert_eq!(resolve_target("hook", Some("https://x")), None);
        assert_eq!(resolve_target("", Some("https://x")), None);
    }

    #[test]
    fn subscription_rules() {
        let ok = SubscriptionDefinition::make("a", "A", "/h").with_connection_code("c");
        let out = subscriptions("orders", &[ok], Some("https://x.example")).unwrap();
        assert_eq!(out[0].target, "https://x.example/h");
        let shared = SubscriptionDefinition::make("a", "A", "https://x.example/h")
            .with_shared_connection(true);
        assert!(
            subscriptions("orders", &[shared], None)
                .unwrap_err()
                .contains("connectionCode")
        );
        let nopath = SubscriptionDefinition::make("z", "Z", "/h");
        assert!(
            subscriptions("orders", &[nopath], None)
                .unwrap_err()
                .contains("z")
        );
    }
}
