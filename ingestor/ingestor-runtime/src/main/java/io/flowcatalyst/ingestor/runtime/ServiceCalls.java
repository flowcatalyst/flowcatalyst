package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.httpclient.ClientResponse;
import io.flowcatalyst.framework.httpclient.HttpClientException;
import io.flowcatalyst.framework.httpclient.RequestBuilder;
import java.io.IOException;

/** Sends a request with the framework's client, turning its failures into the checked ones callers here handle. */
final class ServiceCalls {

  private ServiceCalls() {}

  /**
   * Sends the request.
   *
   * @param request the request
   * @return the answer, whatever its status
   * @throws IOException when there is no answer
   * @throws InterruptedException when interrupted while waiting
   */
  static ClientResponse send(RequestBuilder request) throws IOException, InterruptedException {
    try {
      return request.send();
    } catch (HttpClientException e) {
      if (e.kind() == HttpClientException.Kind.INTERRUPTED) {
        // The client set the flag again; the exception carries it from here.
        Thread.interrupted();
        throw new InterruptedException(e.getMessage());
      }
      throw new IOException(e.getMessage(), e);
    }
  }
}
