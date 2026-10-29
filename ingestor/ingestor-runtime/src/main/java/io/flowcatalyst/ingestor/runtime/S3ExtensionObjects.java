package io.flowcatalyst.ingestor.runtime;

import java.io.IOException;
import java.io.InputStream;
import java.net.URI;
import org.jspecify.annotations.Nullable;
import software.amazon.awssdk.core.exception.SdkException;
import software.amazon.awssdk.services.s3.S3Client;
import software.amazon.awssdk.services.s3.S3ClientBuilder;
import software.amazon.awssdk.services.s3.model.GetObjectRequest;
import software.amazon.awssdk.services.s3.model.NoSuchKeyException;
import software.amazon.awssdk.services.s3.model.S3Exception;

/**
 * {@link ExtensionObjects} over S3, with the default credentials provider chain (the task role) and
 * the default region provider chain. The client's own retry strategy covers a flaky network, a few
 * bounded attempts, before a failure is reported.
 */
final class S3ExtensionObjects implements ExtensionObjects {

  private final S3Client client;

  /**
   * @param endpoint a stand-in for AWS (LocalStack), or null for AWS
   */
  S3ExtensionObjects(@Nullable URI endpoint) {
    S3ClientBuilder builder = S3Client.builder();
    if (endpoint != null) {
      // A stand-in has no virtual-host DNS for the bucket.
      builder.endpointOverride(endpoint).forcePathStyle(true);
    }
    this.client = builder.build();
  }

  @Override
  public InputStream open(String bucket, String key) throws IOException {
    try {
      return client.getObject(GetObjectRequest.builder().bucket(bucket).key(key).build());
    } catch (NoSuchKeyException e) {
      throw new Missing("s3://" + bucket + "/" + key + " does not exist");
    } catch (S3Exception e) {
      // A missing bucket is also a 404, but it is a mistake in the setting, not an unpublished manifest.
      boolean noBucket = e.awsErrorDetails() != null && "NoSuchBucket".equals(e.awsErrorDetails().errorCode());
      if (e.statusCode() == 404 && !noBucket) {
        throw new Missing("s3://" + bucket + "/" + key + " does not exist");
      }
      throw new IOException("s3://" + bucket + "/" + key + " cannot be read: " + e.getMessage(), e);
    } catch (SdkException e) {
      throw new IOException("s3://" + bucket + "/" + key + " cannot be read: " + e.getMessage(), e);
    }
  }
}
