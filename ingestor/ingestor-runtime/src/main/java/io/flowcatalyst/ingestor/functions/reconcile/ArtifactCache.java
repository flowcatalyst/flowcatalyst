package io.flowcatalyst.ingestor.functions.reconcile;

import io.flowcatalyst.ingestor.functions.control.ControlPlane;
import io.flowcatalyst.ingestor.functions.control.ControlPlaneException;
import io.flowcatalyst.ingestor.functions.control.HttpControlPlane;
import java.io.IOException;
import java.io.UncheckedIOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.StandardCopyOption;
import java.util.Comparator;
import java.util.Objects;
import java.util.regex.Pattern;
import java.util.stream.Stream;
import org.jspecify.annotations.Nullable;

/// Keeps downloaded jars on disk by digest (`artifactCache` in the Go runner). The control plane verifies the
/// digest of every download; a cached file is verified again when it is reused (a corrupt file is fetched
/// again). The jars live in a directory of their own, removed on [#close]: a loaded function holds its jar open,
/// so a file is never deleted while a version is loaded from it before the host stops.
public final class ArtifactCache implements AutoCloseable {

    private static final Pattern DIGEST = Pattern.compile("[0-9a-f]{64}");

    private final ControlPlane controlPlane;
    private final Path dir;
    private final boolean ownsDir;

    /// @param dir where the jars are kept, or `null` for a new temporary directory (removed on close)
    public ArtifactCache(ControlPlane controlPlane, @Nullable Path dir) {
        this.controlPlane = Objects.requireNonNull(controlPlane, "controlPlane");
        try {
            if (dir == null) {
                this.dir = Files.createTempDirectory("fn-artifacts-");
                this.ownsDir = true;
            } else {
                Files.createDirectories(dir);
                this.dir = dir;
                this.ownsDir = false;
            }
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    /// The jar for `digest`, downloaded if not kept.
    ///
    /// @throws ControlPlaneException the download failed ([ControlPlaneException#isTransient()] says whether to retry)
    public Path get(String digest) throws ControlPlaneException {
        if (!DIGEST.matcher(digest).matches()) {
            throw new ControlPlaneException(0, "", "digest is not a sha256 hex digest", null, true);
        }
        Path file = dir.resolve(digest + ".jar");
        if (Files.isRegularFile(file)) {
            try {
                if (HttpControlPlane.sha256Hex(Files.readAllBytes(file)).equals(digest)) {
                    return file;
                }
                Files.deleteIfExists(file); // corrupt on disk: fetch again
            } catch (IOException e) {
                // fetch again below
            }
        }
        byte[] bytes = controlPlane.artifact(digest);
        try {
            Path tmp = Files.createTempFile(dir, ".dl-", ".tmp");
            Files.write(tmp, bytes);
            Files.move(tmp, file, StandardCopyOption.REPLACE_EXISTING, StandardCopyOption.ATOMIC_MOVE);
            return file;
        } catch (IOException e) {
            throw new ControlPlaneException(0, "IO", "the artifact could not be stored: " + e.getClass().getSimpleName(), e);
        }
    }

    @Override
    public void close() {
        if (!ownsDir) {
            return;
        }
        try (Stream<Path> walk = Files.walk(dir)) {
            walk.sorted(Comparator.reverseOrder()).forEach(p -> {
                try {
                    Files.deleteIfExists(p);
                } catch (IOException e) {
                    // best effort
                }
            });
        } catch (IOException e) {
            // best effort
        }
    }
}
