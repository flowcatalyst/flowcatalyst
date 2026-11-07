package io.flowcatalyst.ingestor.runtime;

import java.io.ByteArrayOutputStream;
import java.io.OutputStream;
import java.io.PrintStream;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * What the runtime's logging backend (slf4j-simple, which writes to {@code System.err} as it is when a
 * line is written) writes while this is open, read back as records; the lines still reach the console.
 * Stack-trace lines belong to the record before them and are not records.
 */
public final class LogCapture implements AutoCloseable {

  /** slf4j-simple's line: {@code [thread] LEVEL logger - message}. */
  private static final Pattern RECORD = Pattern.compile("^\\[(.*)\\] (TRACE|DEBUG|INFO|WARN|ERROR) ([\\w.$-]+) - (.*)$");

  /**
   * One line the backend wrote.
   *
   * @param thread the thread's name
   * @param level its level
   * @param logger the logger's name
   * @param message the message, without the stack trace
   */
  public record Record(String thread, String level, String logger, String message) {
    /** Whether it is a WARN or an ERROR. */
    public boolean warnOrError() {
      return level.equals("WARN") || level.equals("ERROR");
    }
  }

  private final PrintStream previous;
  private final ByteArrayOutputStream bytes = new ByteArrayOutputStream();

  public LogCapture() {
    previous = System.err;
    OutputStream both =
        new OutputStream() {
          @Override
          public void write(int b) {
            synchronized (bytes) {
              bytes.write(b);
            }
            previous.write(b);
          }

          @Override
          public void write(byte[] b, int off, int len) {
            synchronized (bytes) {
              bytes.write(b, off, len);
            }
            previous.write(b, off, len);
          }
        };
    System.setErr(new PrintStream(both, true, StandardCharsets.UTF_8));
  }

  /** The records written so far. */
  public List<Record> records() {
    String text;
    synchronized (bytes) {
      text = bytes.toString(StandardCharsets.UTF_8);
    }
    List<Record> out = new ArrayList<>();
    for (String line : text.split("\\R")) {
      Matcher m = RECORD.matcher(line);
      if (m.matches()) {
        out.add(new Record(m.group(1), m.group(2), m.group(3), m.group(4)));
      }
    }
    return out;
  }

  @Override
  public void close() {
    System.err.flush();
    System.setErr(previous);
  }
}
