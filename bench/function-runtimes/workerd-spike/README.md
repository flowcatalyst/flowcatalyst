# workerd spike files (2026-10-21)

For `docs/function-runtime-workerd-spike-2026-10-21.md`. `npm i workerd` in a scratch directory, then
`cat shim_head.js ../js/dist/function.js shim_tail2.js > worker2.mjs`, start `node runner_stub.mjs`, run
`workerd serve --experimental config2.capnp`, and PUT the module text with `POST /load?v=2`.

Linux run (2026-10-22): `docker run --rm -v $PWD:/work node:24-slim bash /work/run-linux.sh` after copying
`../fixtures/*` and `../js/dist/function.js` next to the scripts (`--platform linux/amd64` for amd64).
`test.mjs` does equivalence, per-call speed, idle RSS, concurrency, and the runaway kill-and-restart.
