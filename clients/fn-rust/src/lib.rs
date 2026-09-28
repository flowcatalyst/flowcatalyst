//! `flowcatalyst-fn`: the Rust guest SDK for FlowCatalyst functions (plan
//! `docs/function-runner-plan.md` §9). Guest ABI v1 glue lives behind
//! `cfg(target_arch = "wasm32")`; everything else is portable and tested
//! with a normal `cargo test` on the host target.
//!
//! ```ignore
//! use flowcatalyst_fn::{App, Context, Error, Request, Response};
//!
//! flowcatalyst_fn::export!(|app: &mut App| {
//!     app.webhook("POST /events/x", handler)
//!        .platform("GET /hello/{name}", hello)
//!        .open("GET /healthz", health)
//!        .subscribe("t:y:z:w", "/events/x")
//!        .schedule("*/5 * * * *", "/events/x")
//!        .config("GREETING")
//!        .secret("K")
//!        .db("main")
//!        .http_allow("api.x.com")
//!        .emits("a:b:c:d");
//! });
//!
//! fn hello(req: &Request, ctx: &Context) -> Result<Response, Error> {
//!     Ok(Response::new(200).with_body(format!("hi {}", req.path_param("name").unwrap_or("")).into_bytes()))
//! }
//! # fn handler(_: &Request, _: &Context) -> Result<Response, Error> { Ok(Response::new(200)) }
//! # fn health(_: &Request, _: &Context) -> Result<Response, Error> { Ok(Response::new(200)) }
//! ```

mod app;
mod context;
mod dispatch;
mod frame;
mod host;
mod route;
mod types;

pub use app::{App, Handler};
pub use context::{CapabilityError, Context, Event, HttpRequest, HttpResponse, Row};
pub use host::{error_code, HostError};
pub use types::{Caller, Error, Invocation, Principal, Request, Response};

#[cfg(not(target_arch = "wasm32"))]
pub use host::{fake_host_calls, set_fake_host, FakeHostFn};

/// Not part of the public API: the plumbing `export!` expands into. Guest
/// code should not call these directly.
#[doc(hidden)]
pub mod internal {
    pub use crate::app::App;
    pub use crate::dispatch::{alloc, describe, handle};

    pub fn build_app<F: FnOnce(&mut App)>(f: F) -> App {
        let mut app = App::new();
        f(&mut app);
        app
    }
}

/// Declares this module's endpoints/subscriptions/schedules/config/secrets/
/// db/http-allow/emits, and generates the four Guest ABI v1 exports
/// (`fc_abi_v1`, `fc_alloc`, `fc_handle`, `fc_describe`; plan §5.1). Call it
/// exactly once, at the crate root of a `crate-type = ["cdylib"]` guest
/// built for `wasm32-unknown-unknown`.
///
/// The `App` is rebuilt fresh on every `fc_handle`/`fc_describe` call
/// (registration is just pattern strings and `fn` pointers, so this is
/// cheap) rather than cached behind global mutable state.
#[macro_export]
macro_rules! export {
    ($build:expr) => {
        #[no_mangle]
        pub extern "C" fn fc_abi_v1() {}

        #[no_mangle]
        pub extern "C" fn fc_alloc(size: u32) -> u32 {
            $crate::internal::alloc(size)
        }

        #[no_mangle]
        pub unsafe extern "C" fn fc_handle(ptr: u32, len: u32) -> u64 {
            $crate::internal::handle($crate::internal::build_app($build), ptr, len)
        }

        #[no_mangle]
        pub extern "C" fn fc_describe() -> u64 {
            $crate::internal::describe($crate::internal::build_app($build))
        }
    };
}
