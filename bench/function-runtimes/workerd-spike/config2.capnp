using Workerd = import "/workerd/workerd.capnp";
const config :Workerd.Config = (
  services = [(name = "main", worker = .w), (name = "runner", external = (address = "127.0.0.1:19090", http = ()))],
  sockets = [(name = "http", address = "127.0.0.1:18787", http = (), service = "main")]
);
const w :Workerd.Worker = (
  modules = [(name = "loader", esModule = embed "loader.mjs")],
  compatibilityDate = "2026-10-01",
  compatibilityFlags = ["experimental"],
  bindings = [(name = "LOADER", workerLoader = (id = "fns")), (name = "RUNNER", service = "runner")],
);
