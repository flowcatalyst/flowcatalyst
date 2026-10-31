package io.flowcatalyst.ingestor.functions.http;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.functions.context.AllowlistHttpCaller;
import io.flowcatalyst.ingestor.functions.context.HostFunctionContext;
import io.flowcatalyst.ingestor.functions.load.FunctionRegistry;
import io.flowcatalyst.ingestor.functions.load.LoadedFunction;
import io.flowcatalyst.ingestor.functions.reconcile.Reconciler;
import java.io.IOException;
import java.io.InputStream;
import java.lang.classfile.ClassFile;
import java.lang.classfile.ClassModel;
import java.lang.classfile.CodeElement;
import java.lang.classfile.MethodModel;
import java.lang.classfile.Opcode;
import java.lang.classfile.instruction.MonitorInstruction;
import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.Test;

/// "No `synchronized` block or single lock on the invoke path" (R3), checked in the compiled classes: none of the
/// classes a call passes through may contain a `monitorenter` (a `synchronized` block or method compiles to one,
/// and a synchronized method carries the flag). The locks that remain are `ReentrantLock`s on the slow paths:
/// loading a version on demand ([Reconciler#resolve] when the registry has no instance, [PinnedVersions#getOrLoad]
/// when none is cached) and registry writes, none of them taken by a call to a loaded version.
class NoLockOnInvokePathTest {

    private static final List<Class<?>> INVOKE_PATH = List.of(
            FunctionListener.class,
            InvocationRunner.class,
            Permits.class,
            Permits.Grant.class,
            EndpointRouter.class,
            WebhookValidator.class,
            CallerMapper.class,
            CorsPolicy.class,
            LoadedFunction.class,
            FunctionRegistry.class,
            Reconciler.class,
            HostFunctionContext.class,
            AllowlistHttpCaller.class,
            io.flowcatalyst.ingestor.functions.FrameworkAccess.class);

    @Test
    void noClassOnTheInvokePathEntersAMonitor() throws IOException {
        List<String> found = new ArrayList<>();
        for (Class<?> c : INVOKE_PATH) {
            ClassModel model = parse(c);
            for (MethodModel method : model.methods()) {
                if (method.flags().has(java.lang.reflect.AccessFlag.SYNCHRONIZED)) {
                    found.add(c.getSimpleName() + "." + method.methodName().stringValue() + " is synchronized");
                }
                method.code().ifPresent(code -> {
                    for (CodeElement e : code) {
                        if (e instanceof MonitorInstruction m && m.opcode() == Opcode.MONITORENTER) {
                            found.add(c.getSimpleName() + "." + method.methodName().stringValue() + " enters a monitor");
                        }
                    }
                });
            }
        }
        assertThat(found).isEmpty();
    }

    @Test
    void theScanCanFail() throws IOException {
        // the control: a class that does synchronize is seen
        ClassModel model = parse(Synchronizes.class);
        long monitors = model.methods().stream()
                .flatMap(m -> m.code().stream())
                .flatMap(code -> code.elementList().stream())
                .filter(e -> e instanceof MonitorInstruction m && m.opcode() == Opcode.MONITORENTER)
                .count();
        assertThat(monitors).isGreaterThan(0);
    }

    static final class Synchronizes {
        private final Object lock = new Object();

        int value() {
            synchronized (lock) {
                return 1;
            }
        }
    }

    private static ClassModel parse(Class<?> c) throws IOException {
        String resource = c.getName().replace('.', '/') + ".class";
        try (InputStream in = c.getClassLoader().getResourceAsStream(resource)) {
            assertThat(in).as(resource).isNotNull();
            return ClassFile.of().parse(in.readAllBytes());
        }
    }
}
