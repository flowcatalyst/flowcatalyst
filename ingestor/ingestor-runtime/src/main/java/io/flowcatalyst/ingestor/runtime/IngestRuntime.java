package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.core.App;
import io.flowcatalyst.framework.core.AppBuilder;
import io.flowcatalyst.framework.core.Launch;
import io.flowcatalyst.framework.flowcatalyst.FlowCatalystModule;
import io.flowcatalyst.framework.http.jetty.JettyHttpModule;
import io.flowcatalyst.framework.httpclient.module.HttpClientModule;

/** The ingestor runtime application. */
public final class IngestRuntime {

  private IngestRuntime() {}

  /**
   * The application: every module it needs, listed in code.
   *
   * @return a builder with the modules added
   */
  public static AppBuilder builder() {
    return App.builder()
        .module(new HttpClientModule())
        // The platform: its base URL and the runtime's service account, whose token every call to the runtime API carries.
        .module(new FlowCatalystModule())
        .module(new RuntimeModule())
        // Last: the port opens only once every other module has started.
        .module(new JettyHttpModule());
  }

  /**
   * Starts the runtime and runs until the process is stopped.
   *
   * @param args passed to the framework
   */
  public static void main(String[] args) {
    switch (builder().shutdownHook(true).launch(args)) {
      case Launch.Ran ran -> System.exit(ran.exitCode());
      case Launch.Started started -> {}
    }
  }
}
