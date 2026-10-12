//! `cargo run --example list-event-types --features client`
//!
//! Smallest possible smoke test of the FlowCatalyst API client. Lists the
//! event types currently registered on the platform. Useful as the
//! "everything's wired correctly" check before reaching for a bigger
//! example.
//!
//! ## Run
//!
//! ```bash
//! export FC_BASE_URL=https://platform.example.com
//! # either a token ...
//! export FC_TOKEN=<a-platform-access-token>
//! # ... or a service account's client credentials
//! export FC_CLIENT_ID=<oauth-client-id> FC_CLIENT_SECRET=<secret>
//! cargo run --example list-event-types --features client
//! ```

use flowcatalyst_sdk::client::{ClientCredentials, FlowCatalystClient};
use std::env;
use std::error;

#[tokio::main]
async fn main() -> Result<(), Box<dyn error::Error>> {
    let base_url = env::var("FC_BASE_URL")?;
    let client = FlowCatalystClient::new(base_url.clone());
    let client = match (env::var("FC_CLIENT_ID"), env::var("FC_CLIENT_SECRET")) {
        (Ok(id), Ok(secret)) => {
            client.with_token_source(ClientCredentials::new(&base_url, id, secret))
        }
        _ => client.with_token(env::var("FC_TOKEN")?),
    };
    let event_types = client.event_types().list(None, None, None).await?;

    println!("Found {} event type(s):", event_types.items.len());
    for et in event_types.items {
        println!("  {:<48}  {}", et.code, et.name);
    }
    Ok(())
}
