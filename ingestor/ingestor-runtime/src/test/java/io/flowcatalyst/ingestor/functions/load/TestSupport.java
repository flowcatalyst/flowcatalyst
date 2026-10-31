package io.flowcatalyst.ingestor.functions.load;

import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.function.FunctionContext;
import io.flowcatalyst.function.Request;
import io.flowcatalyst.function.Result;
import io.flowcatalyst.function.testing.FakeFunctionContext;
import io.flowcatalyst.function.testing.FakeRequest;
import java.nio.charset.StandardCharsets;
import java.nio.file.Path;

/// Shared fixtures for the loader tests: a stock address, a throwaway [Request] and a context.
final class TestSupport {

    static final FunctionAddress ADDRESS = FunctionAddress.parse("fixture.probe");

    private TestSupport() {
    }

    static Request request() {
        return FakeRequest.post("/").build();
    }

    static FunctionContext context() {
        return FakeFunctionContext.builder(ADDRESS).build();
    }

    static Path tempJar(Path dir, String name) {
        return dir.resolve(name + ".jar");
    }

    /// Extracts the `reason` a fixture handed to `Result.fail(reason)` back out of the JSON body
    /// `Result.fail` produces (`{"error":"<reason>"}`).
    static String failReason(Result result) {
        String json = new String(result.body(), StandardCharsets.UTF_8);
        int start = json.indexOf(":\"") + 2;
        int end = json.lastIndexOf('"');
        return json.substring(start, end).replace("\\\"", "\"").replace("\\\\", "\\");
    }

    /// Whether `result` is [Result#ack()] — status 200 with an empty body.
    static boolean isAck(Result result) {
        return result.status() == 200 && result.body().length == 0;
    }
}
