package io.flowcatalyst.ingestor.runtime;

import java.net.StandardProtocolFamily;
import java.net.UnixDomainSocketAddress;
import java.nio.ByteBuffer;
import java.nio.channels.SocketChannel;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;

/**
 * Whether a Docker daemon answers, for the tests that start real servers (SFTP, RabbitMQ, a local
 * AWS, PostgreSQL). Those tests are skipped, not failed, when it does not: a daemon that is installed
 * but hung must not hang the build, so it is asked with a time limit and Testcontainers is not asked at all.
 */
final class Docker {

  private static final Boolean AVAILABLE = ping();

  private Docker() {}

  /** For {@code @EnabledIf}. */
  static boolean available() {
    return AVAILABLE;
  }

  private static boolean ping() {
    String host = System.getenv("DOCKER_HOST");
    Path socket = host != null && host.startsWith("unix://") ? Path.of(host.substring("unix://".length())) : Path.of("/var/run/docker.sock");
    if (!Files.exists(socket)) {
      return false;
    }
    CompletableFuture<Boolean> asked =
        CompletableFuture.supplyAsync(
            () -> {
              try (SocketChannel channel = SocketChannel.open(StandardProtocolFamily.UNIX)) {
                channel.connect(UnixDomainSocketAddress.of(socket));
                channel.write(ByteBuffer.wrap("GET /_ping HTTP/1.0\r\n\r\n".getBytes(StandardCharsets.US_ASCII)));
                ByteBuffer answer = ByteBuffer.allocate(64);
                channel.read(answer);
                return new String(answer.array(), 0, answer.position(), StandardCharsets.US_ASCII).startsWith("HTTP/1.");
              } catch (java.io.IOException e) {
                return false;
              }
            });
    try {
      return asked.get(3, TimeUnit.SECONDS);
    } catch (java.util.concurrent.TimeoutException e) {
      asked.cancel(true);
      return false;
    } catch (InterruptedException e) {
      Thread.currentThread().interrupt();
      return false;
    } catch (java.util.concurrent.ExecutionException e) {
      return false;
    }
  }
}
