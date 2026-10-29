package io.flowcatalyst.ingestor.runtime;

import java.io.IOException;
import java.io.InputStream;

/** Reads objects from a bucket: the seam that keeps the manifest loader testable without AWS. */
interface ExtensionObjects {

  /** The object does not exist: for the manifest, nothing has been published yet. */
  final class Missing extends IOException {
    private static final long serialVersionUID = 1L;

    Missing(String message) {
      super(message);
    }
  }

  /**
   * Opens an object for reading.
   *
   * @param bucket the bucket
   * @param key the object's key
   * @return its content; the caller closes it
   * @throws Missing when the object does not exist
   * @throws IOException when it cannot be read (access denied, the network after the client's own retries)
   */
  InputStream open(String bucket, String key) throws IOException;
}
