package io.flowcatalyst.ingestor.functions.context;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.function.DispatchException;
import io.flowcatalyst.function.DispatchJob;
import io.flowcatalyst.function.EmitResult;
import io.flowcatalyst.function.OutboundEvent;
import io.flowcatalyst.ingestor.functions.FakeControlPlane;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.Describe;
import java.time.Clock;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/// The context a function receives: config and secrets limited to the declared keys (their values never printed),
/// databases not available yet (a clear refusal), events and dispatches limited to what the describe declares.
class HostFunctionContextTest {

    HttpClients clients;
    FakeControlPlane plane;
    HostFunctionContext ctx;

    @BeforeEach
    void start() {
        clients = HttpClients.create();
        plane = new FakeControlPlane();
        Control.FunctionDoc doc = new Control.FunctionDoc("fn_1", "app.hello", "app_1", null, null, false, null,
                Map.of("a", "1", "b", "2"), Map.of("s", "hunter2", "t", "other"), Map.of("main", "postgres://u:p@h/db"), List.of());
        Describe describe = new Describe(1, List.of(), List.of("a"), List.of("s"), List.of("main"), List.of(), List.of("app:orders:order:created"),
                List.of(new Describe.DispatchTarget("app.other", "/hook", null, "conn")), new Describe.Jvm("a.B"));
        ctx = new ContextFactory(new OutboundClients(clients), Clock.systemUTC(), plane).build(doc, 4, describe);
    }

    @AfterEach
    void stop() {
        clients.close();
    }

    @Test
    void configAndSecretsAreLimitedToTheDeclaredKeys() {
        assertThat(ctx.config().get("a")).contains("1");
        assertThat(ctx.config().get("b")).as("set on the platform but not declared").isEmpty();
        assertThat(ctx.secrets().get("s")).contains("hunter2");
        assertThat(ctx.secrets().get("t")).isEmpty();
        assertThatThrownBy(() -> ctx.config().require("b")).isInstanceOf(java.util.NoSuchElementException.class).hasMessageContaining("b");
        assertThatThrownBy(() -> ctx.secrets().require("t")).isInstanceOf(java.util.NoSuchElementException.class);
    }

    @Test
    void aSecretIsNeverPrinted() {
        assertThat(ctx.secrets().toString()).contains("s").doesNotContain("hunter2");
        assertThat(ctx.config().toString()).doesNotContain("1");
    }

    @Test
    void theContextCarriesTheAddressAndVersion() {
        assertThat(ctx.address().render()).isEqualTo("app.hello");
        assertThat(ctx.version()).isEqualTo(4);
        assertThat(ctx.clock()).isNotNull();
        assertThat(ctx.logger().getName()).isEqualTo("fn.app.hello");
    }

    @Test
    void aDeclaredDatabaseIsRefusedClearlyAndAnUndeclaredOneIsAnArgumentError() {
        assertThatThrownBy(() -> ctx.dataSource("main"))
                .isInstanceOf(UnsupportedOperationException.class)
                .hasMessageContaining("main")
                .hasMessageContaining("database support");
        assertThatThrownBy(() -> ctx.dataSource("other")).isInstanceOf(IllegalArgumentException.class);
    }

    @Test
    void anEventOfADeclaredTypeIsSentAndOthersAreRefusedWithoutACall() {
        OutboundEvent ok = new OutboundEvent("app:orders:order:created", null, null, null, new byte[0], null, null, null, "d1");
        OutboundEvent other = new OutboundEvent("app:orders:order:shipped", null, null, null, new byte[0], null, null, null, "d2");

        assertThat(ctx.events().emit(ok)).isInstanceOf(EmitResult.Emitted.class);
        EmitResult refused = ctx.events().emit(other);

        assertThat(refused).isInstanceOfSatisfying(EmitResult.Refused.class, r -> {
            assertThat(r.code()).isEqualTo("NOT_DECLARED");
            assertThat(r.retryable()).isFalse();
        });
        assertThat(plane.emits).hasSize(1);
        assertThat(plane.emits.get(0).event().source()).isEqualTo("function:app.hello");
    }

    @Test
    void aDispatchToADeclaredTargetIsSentAndOthersAreRefusedWithoutACall() {
        assertThat(ctx.dispatcher().dispatch(DispatchJob.to("app.other", "/hook").payload("x").build())).isEqualTo("djb_1");
        assertThatThrownBy(() -> ctx.dispatcher().dispatch(DispatchJob.to("app.other", "/elsewhere").build()))
                .isInstanceOfSatisfying(DispatchException.class, e -> assertThat(e.code()).isEqualTo("NOT_DECLARED"));
        assertThat(plane.dispatches).hasSize(1);
    }

    @Test
    void aDispatchPayloadOver256KiBIsRefused() {
        assertThatThrownBy(() -> ctx.dispatcher().dispatch(DispatchJob.to("app.other", "/hook").payload(new byte[(256 << 10) + 1]).build()))
                .isInstanceOfSatisfying(DispatchException.class, e -> assertThat(e.code()).isEqualTo("TOO_LARGE"));
        assertThat(plane.dispatches).isEmpty();
    }

    @Test
    void anAddressTheFunctionApiCannotRepresentIsRefusedAtBuild() {
        Control.FunctionDoc doc = new Control.FunctionDoc("fn_1", "App_1.hello", "app_1", null, null, false, null, Map.of(), Map.of(), Map.of(), List.of());
        Describe describe = new Describe(1, List.of(), List.of(), List.of(), List.of(), List.of(), List.of(), List.of(), new Describe.Jvm("a.B"));
        assertThatThrownBy(() -> new ContextFactory(new OutboundClients(clients), Clock.systemUTC(), plane).build(doc, 1, describe))
                .isInstanceOfSatisfying(ContextLoadException.class, e -> {
                    assertThat(e.code()).isEqualTo("ADDRESS_UNSUPPORTED");
                    assertThat(e.isTransient()).isFalse();
                });
    }
}
