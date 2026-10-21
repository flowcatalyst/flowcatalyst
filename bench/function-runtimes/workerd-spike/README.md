# workerd spike files (2026-10-21)

For `docs/function-runtime-workerd-spike-2026-10-21.md`. `npm i workerd` in a scratch directory, then
`cat shim_head.js ../js/dist/function.js shim_tail2.js > worker2.mjs`, start `node runner_stub.mjs`, run
`workerd serve --experimental config2.capnp`, and PUT the module text with `POST /load?v=2`.
