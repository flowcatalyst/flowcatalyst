package io.flowcatalyst.ingestor.functions.context;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.nio.charset.StandardCharsets;
import java.text.MessageFormat;
import java.util.Arrays;
import java.util.MissingResourceException;
import java.util.Objects;
import java.util.ResourceBundle;
import org.jspecify.annotations.Nullable;

/// A [System.Logger] backed by the SLF4J logger `fn.<address>` (docs/jvm-functions-in-ingestor.md §3). `System.Logger` has exactly four abstract
/// methods — `getName`, `isLoggable`, and the two `ResourceBundle`-taking
/// `log` overloads — every other overload (`log(Level, String)`,
/// `log(Level, String, Object...)`, the `Supplier` forms, …) is a JDK
/// default method that funnels into these, so implementing only these four
/// covers the whole interface.
///
/// Level mapping: `TRACE`/`DEBUG`/`INFO`/`WARNING`/`ERROR` map to the
/// matching SLF4J level; `ALL` logs at the lowest SLF4J level (`trace`) —
/// never filtered out, matching what `isLoggable(ALL)` promises; `OFF` is
/// never loggable, so [#log] never even reaches the emit step for it.
///
/// A function's log is bounded (docs/ingestor-failure-pacing.md R6): every line that passes the level check
/// takes one from the address's [LogBudget] (100 a second, burst 1,000, by default) and is dropped when there is
/// none; a message over [#MAX_MESSAGE] is cut; and a minute's drops are reported as one WARN from the host.
public final class HostLogger implements System.Logger {

    /// The longest message kept, in characters; longer is cut and says how many bytes it was.
    static final int MAX_MESSAGE = 8 * 1024;

    private final Logger slf4j;
    private final String address;
    private final LogBudget budget;

    public HostLogger(String address) {
        this(address, new LogBudget(LogBudget.Limits.DEFAULTS));
    }

    /// @param budget the address's budget, shared by all its versions
    public HostLogger(String address, LogBudget budget) {
        this(LoggerFactory.getLogger("fn." + Objects.requireNonNull(address, "address")), address, budget);
    }

    /// Test seam: log against an arbitrary SLF4J logger (a fixed name, so a
    /// test's `ListAppender` can be attached to it directly).
    HostLogger(Logger slf4j) {
        this(slf4j, slf4j.getName(), new LogBudget(LogBudget.Limits.DEFAULTS));
    }

    HostLogger(Logger slf4j, String address, LogBudget budget) {
        this.slf4j = Objects.requireNonNull(slf4j, "slf4j");
        this.address = Objects.requireNonNull(address, "address");
        this.budget = Objects.requireNonNull(budget, "budget");
    }

    @Override
    public String getName() {
        return slf4j.getName();
    }

    @Override
    public boolean isLoggable(Level level) {
        return switch (level) {
            case ALL -> true;
            case TRACE -> slf4j.isTraceEnabled();
            case DEBUG -> slf4j.isDebugEnabled();
            case INFO -> slf4j.isInfoEnabled();
            case WARNING -> slf4j.isWarnEnabled();
            case ERROR -> slf4j.isErrorEnabled();
            case OFF -> false;
        };
    }

    @Override
    public void log(Level level, ResourceBundle bundle, String msg, Throwable thrown) {
        if (!isLoggable(level)) {
            return;
        }
        emit(level, resolve(bundle, msg), thrown);
    }

    @Override
    public void log(Level level, ResourceBundle bundle, String format, Object... params) {
        if (!isLoggable(level)) {
            return;
        }
        String resolved = resolve(bundle, format);
        Throwable cause = null;
        Object[] args = params;
        // System.Logger's own contract for this overload: a trailing Throwable
        // is the record's cause, not a format argument (spec: "a throwable goes
        // to setCause").
        if (params != null && params.length > 0 && params[params.length - 1] instanceof Throwable t) {
            cause = t;
            args = Arrays.copyOf(params, params.length - 1);
        }
        String formatted = (resolved != null && args != null && args.length > 0)
                ? MessageFormat.format(resolved, args)
                : resolved;
        emit(level, formatted, cause);
    }

    private static @Nullable String resolve(@Nullable ResourceBundle bundle, @Nullable String key) {
        if (bundle == null || key == null) {
            return key;
        }
        try {
            return bundle.getString(key);
        } catch (MissingResourceException e) {
            return key;
        }
    }

    private void emit(Level level, @Nullable String message, @Nullable Throwable cause) {
        long dropped = budget.takeDropped();
        if (dropped > 0) {
            slf4j.warn("function {} dropped {} log lines in the last minute (limit {}/s)", address, dropped, budget.limits().rate());
        }
        if (!budget.tryAcquire()) {
            return;
        }
        message = cut(message);
        if (cause != null && cause.getMessage() != null && cause.getMessage().length() > MAX_MESSAGE) {
            cause = new CutThrowable(cause);
        }
        switch (level) {
            case TRACE, ALL -> log(slf4j::trace, slf4j::trace, message, cause);
            case DEBUG -> log(slf4j::debug, slf4j::debug, message, cause);
            case INFO -> log(slf4j::info, slf4j::info, message, cause);
            case WARNING -> log(slf4j::warn, slf4j::warn, message, cause);
            case ERROR -> log(slf4j::error, slf4j::error, message, cause);
            case OFF -> {
                // unreachable: isLoggable(OFF) is always false, so callers never get here.
            }
        }
    }

    /// `message` cut to [#MAX_MESSAGE] characters (not in the middle of a surrogate pair), ending "… (cut, M bytes)"
    /// where M is its size in UTF-8.
    static @Nullable String cut(@Nullable String message) {
        if (message == null || message.length() <= MAX_MESSAGE) {
            return message;
        }
        int end = MAX_MESSAGE;
        if (Character.isHighSurrogate(message.charAt(end - 1))) {
            end--;
        }
        return message.substring(0, end) + "… (cut, " + message.getBytes(StandardCharsets.UTF_8).length + " bytes)";
    }

    /// A throwable whose message is over the limit, logged in its place: the original's type and stack trace, its
    /// message cut. (Its causes' messages are not cut; the stack trace is the long part of a throwable.)
    private static final class CutThrowable extends Throwable {
        private static final long serialVersionUID = 1L;

        CutThrowable(Throwable original) {
            super(original.getClass().getName() + ": " + cut(original.getMessage()), original.getCause(), false, true);
            setStackTrace(original.getStackTrace());
        }
    }

    private interface Plain {
        void log(@Nullable String message);
    }

    private interface WithCause {
        void log(@Nullable String message, Throwable cause);
    }

    private static void log(Plain plain, WithCause withCause, @Nullable String message, @Nullable Throwable cause) {
        if (cause != null) {
            withCause.log(message, cause);
        } else {
            plain.log(message);
        }
    }
}
