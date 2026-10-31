package io.flowcatalyst.function.testing;

import java.text.MessageFormat;
import java.util.List;
import java.util.ResourceBundle;
import java.util.concurrent.CopyOnWriteArrayList;

/// A [System.Logger] that captures every fully formatted line, so a test can
/// assert what a function logged (or that a secret is absent from the log).
/// `System.Logger` declares only four abstract methods; every other overload,
/// including the `Object...` one a function usually calls, delegates to them,
/// so implementing these observes everything. Formats with [MessageFormat],
/// as a real host's logger does.
public final class CapturingLogger implements System.Logger {

    private final String name;
    private final List<String> lines = new CopyOnWriteArrayList<>();

    public CapturingLogger(String name) {
        this.name = name;
    }

    @Override
    public String getName() {
        return name;
    }

    @Override
    public boolean isLoggable(Level level) {
        return true;
    }

    @Override
    public void log(Level level, ResourceBundle bundle, String msg, Throwable thrown) {
        lines.add(thrown == null ? msg : msg + " " + thrown);
    }

    @Override
    public void log(Level level, ResourceBundle bundle, String format, Object... params) {
        lines.add(params == null || params.length == 0 ? format : MessageFormat.format(format, params));
    }

    /// Every line logged so far, formatted, in order.
    public List<String> lines() {
        return lines;
    }
}
