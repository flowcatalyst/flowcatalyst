package io.flowcatalyst.function;

import java.util.regex.Pattern;

/// A function's address, `{applicationCode}.{name}` — two labels. It is the
/// identity a function reads off every [Request] and passes to
/// [FunctionContext#address()], and the `{address}` of the invocation path
/// `/fn/{address}[@{alias}|@v{n}]/{path}`.
///
/// This restates the platform's rules (`internal/platform/function`
/// `BuildAddress`, `NamePattern` and the application-code rule): the API jar
/// has zero dependencies, so it cannot share the platform's types.
///
/// - `application` is the owning application's code: a lower-case letter,
///   then lower-case letters, digits, `_` and `-` (the platform's
///   application-code rule, `validate.CodeUnderscorePattern`).
/// - `name` is the function's own name: a lower-case letter, then up to 62
///   lower-case letters, digits and `-` (63 characters at most).
///
/// No normalisation: the input is not trimmed and not lower-cased, and
/// [#render()] gives back exactly what [#parse(String)] accepted.
///
/// @param application the owning application's code
/// @param name        the function's name within the application
public record FunctionAddress(String application, String name) {

    private static final Pattern APPLICATION = Pattern.compile("^[a-z][a-z0-9_-]*$");
    private static final Pattern NAME = Pattern.compile("^[a-z][a-z0-9-]{0,62}$");
    private static final String MESSAGE =
            "address must be {applicationCode}.{name}: two labels separated by '.'";

    public FunctionAddress {
        if (application == null || !APPLICATION.matcher(application).matches()) {
            throw new IllegalArgumentException(MESSAGE + " (bad application code: " + application + ")");
        }
        if (name == null || !NAME.matcher(name).matches()) {
            throw new IllegalArgumentException(MESSAGE + " (bad name: " + name + ")");
        }
    }

    /// Splits on `.` and requires exactly two segments, each valid by the
    /// rules above.
    ///
    /// @throws IllegalArgumentException for any failure: wrong segment count,
    ///                                  an empty or malformed segment, or a
    ///                                  `null` input
    public static FunctionAddress parse(String raw) {
        if (raw == null) throw new IllegalArgumentException(MESSAGE + " (null)");
        int dot = raw.indexOf('.');
        if (dot < 0 || raw.indexOf('.', dot + 1) >= 0) {
            throw new IllegalArgumentException(MESSAGE + ": " + raw);
        }
        return new FunctionAddress(raw.substring(0, dot), raw.substring(dot + 1));
    }

    /// The dotted form, `application.name`.
    public String render() {
        return application + "." + name;
    }

    @Override
    public String toString() {
        return render();
    }
}
