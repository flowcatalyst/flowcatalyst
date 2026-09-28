// fn.log (host op 1) and the console shim (plan: "also install
// globalThis.console (log/info/warn/error/debug) routed to it"), mirroring
// clients/fn-go/log.go's slog.Handler.
import { callHost } from "./native";
import { OpLog } from "./ops";

export type LogLevel = "DEBUG" | "INFO" | "WARN" | "ERROR";

function logOp(level: LogLevel, msg: string, attrs?: Record<string, unknown>): void {
  try {
    // Logging must never fail a handler: swallow any host-reported failure,
    // same as clients/fn-go/log.go's slogHandler.
    callHost(OpLog, { level, msg, attrs });
  } catch {
    // ignored
  }
}

export const log = {
  debug(msg: string, attrs?: Record<string, unknown>): void {
    logOp("DEBUG", msg, attrs);
  },
  info(msg: string, attrs?: Record<string, unknown>): void {
    logOp("INFO", msg, attrs);
  },
  warn(msg: string, attrs?: Record<string, unknown>): void {
    logOp("WARN", msg, attrs);
  },
  error(msg: string, attrs?: Record<string, unknown>): void {
    logOp("ERROR", msg, attrs);
  },
};

function stringifyArg(a: unknown): string {
  if (typeof a === "string") return a;
  if (a instanceof Error) return a.stack ?? a.message;
  try {
    return JSON.stringify(a);
  } catch {
    return String(a);
  }
}

function formatConsoleArgs(args: unknown[]): string {
  return args.map(stringifyArg).join(" ");
}

export interface ConsoleLike {
  log(...args: unknown[]): void;
  info(...args: unknown[]): void;
  warn(...args: unknown[]): void;
  error(...args: unknown[]): void;
  debug(...args: unknown[]): void;
}

/** Installs a console-shaped object routed through op 1 onto `target`
 * (defaults to globalThis). console.log/info map to INFO, matching common
 * runtime convention. */
export function installConsole(target: Record<string, unknown> = globalThis as unknown as Record<string, unknown>): void {
  const c: ConsoleLike = {
    log: (...args) => logOp("INFO", formatConsoleArgs(args)),
    info: (...args) => logOp("INFO", formatConsoleArgs(args)),
    warn: (...args) => logOp("WARN", formatConsoleArgs(args)),
    error: (...args) => logOp("ERROR", formatConsoleArgs(args)),
    debug: (...args) => logOp("DEBUG", formatConsoleArgs(args)),
  };
  target.console = c;
}
