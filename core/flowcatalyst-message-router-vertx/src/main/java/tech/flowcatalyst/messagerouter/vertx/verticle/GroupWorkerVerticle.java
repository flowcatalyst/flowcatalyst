package tech.flowcatalyst.messagerouter.vertx.verticle;

import io.vertx.core.AbstractVerticle;
import io.vertx.core.eventbus.Message;
import io.vertx.core.json.JsonObject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.vertx.channel.MediatorChannels;
import tech.flowcatalyst.messagerouter.vertx.channel.RouterChannels;
import tech.flowcatalyst.messagerouter.vertx.message.RouterMessages.*;

/**
 * Worker verticle for processing messages in a single message group.
 * <p>
 * One instance per active message group. Single-threaded, no locks needed.
 * Guarantees FIFO ordering within the group by processing sequentially.
 * <p>
 * Lifecycle:
 * <ul>
 *   <li>Deployed by PoolVerticle when first message for group arrives</li>
 *   <li>Processes messages sequentially</li>
 *   <li>Signals idle after timeout (no messages for 5 minutes)</li>
 *   <li>Undeployed by PoolVerticle when idle</li>
 * </ul>
 * <p>
 * Threading: Virtual Thread (blocking mediator calls are fine)
 */
public class GroupWorkerVerticle extends AbstractVerticle {

    private static final Logger LOG = Logger.getLogger(GroupWorkerVerticle.class);
    private static final long IDLE_TIMEOUT_MS = 5 * 60 * 1000; // 5 minutes

    private String poolCode;
    private String groupId;
    private long lastActivityTime;
    private long idleCheckTimerId;
    private boolean processing = false;

    @Override
    public void start() {
        JsonObject cfg = config();
        this.poolCode = cfg.getString("poolCode");
        this.groupId = cfg.getString("groupId");
        this.lastActivityTime = System.currentTimeMillis();

        LOG.debugf("GroupWorkerVerticle starting for pool [%s] group [%s]", poolCode, groupId);

        // Listen for messages on group-specific address
        String address = "pool." + poolCode + ".group." + groupId;
        vertx.eventBus().<PoolMessage>consumer(address, this::handleMessage);

        // Periodic idle check
        idleCheckTimerId = vertx.setPeriodic(60_000, id -> checkIdle());

        LOG.infof("GroupWorkerVerticle started for pool [%s] group [%s]", poolCode, groupId);
    }

    @Override
    public void stop() {
        vertx.cancelTimer(idleCheckTimerId);
        LOG.infof("GroupWorkerVerticle stopped for pool [%s] group [%s]", poolCode, groupId);
    }

    private void handleMessage(Message<PoolMessage> msg) {
        lastActivityTime = System.currentTimeMillis();
        PoolMessage message = msg.body();

        LOG.debugf("Processing message [%s] in group [%s]", message.sqsMessageId(), groupId);

        processing = true;
        try {
            processMessage(message);
        } finally {
            processing = false;
        }

        // Acknowledge receipt to pool
        msg.reply(new OkReply());
    }

    private void processMessage(PoolMessage message) {
        long startTime = System.currentTimeMillis();

        try {
            // Build mediation request
            MediationRequest request = new MediationRequest(
                    message.id(),
                    message.sqsMessageId(),
                    message.authToken(),
                    message.mediationType(),
                    message.mediationTarget(),
                    message.messageGroupId()
            );

            // Blocking request-reply to mediator (fine on virtual thread)
            MediationResult result = MediatorChannels.address(poolCode)
                    .mediate(vertx)
                    .requestBlocking(request);

            long durationMs = System.currentTimeMillis() - startTime;
            handleMediationResult(message, result, durationMs);

        } catch (Exception e) {
            long durationMs = System.currentTimeMillis() - startTime;
            LOG.warnf("Mediation failed for message [%s]: %s", message.sqsMessageId(), e.getMessage());
            sendNack(message.sqsMessageId(), 30);
            notifyBatchGroupFailed(message);
        }
    }

    private void handleMediationResult(PoolMessage message, MediationResult result, long durationMs) {
        switch (result.outcome()) {
            case SUCCESS -> {
                LOG.debugf("Message [%s] processed successfully in %dms", message.sqsMessageId(), durationMs);
                sendAck(message.sqsMessageId());
            }
            case NACK -> {
                LOG.debugf("Message [%s] NACKed with delay %ds", message.sqsMessageId(), result.delaySeconds());
                sendNack(message.sqsMessageId(), result.delaySeconds());
                notifyBatchGroupFailed(message);
            }
            case ERROR_CONFIG -> {
                // Configuration error - ACK to remove from queue (won't succeed on retry)
                LOG.warnf("Message [%s] has config error, ACKing to remove", message.sqsMessageId());
                sendAck(message.sqsMessageId());
            }
        }
    }

    private void sendAck(String sqsMessageId) {
        try {
            RouterChannels.ack(vertx).requestBlocking(new AckRequest(sqsMessageId));
        } catch (Exception e) {
            LOG.errorf("Failed to send ACK for [%s]: %s", sqsMessageId, e.getMessage());
        }
    }

    private void sendNack(String sqsMessageId, int delaySeconds) {
        try {
            RouterChannels.nack(vertx).requestBlocking(new NackRequest(sqsMessageId, delaySeconds));
        } catch (Exception e) {
            LOG.errorf("Failed to send NACK for [%s]: %s", sqsMessageId, e.getMessage());
        }
    }

    private void notifyBatchGroupFailed(PoolMessage message) {
        // Notify pool that this batch+group failed (for FIFO failure handling)
        String address = "pool." + poolCode + ".batchGroupFailed";
        vertx.eventBus().send(address, new BatchGroupFailed(message.batchId(), groupId));
    }

    private void checkIdle() {
        if (processing) {
            return; // Currently processing, not idle
        }

        long idleTime = System.currentTimeMillis() - lastActivityTime;
        if (idleTime >= IDLE_TIMEOUT_MS) {
            LOG.infof("GroupWorkerVerticle for pool [%s] group [%s] idle for %dms, requesting undeploy",
                    poolCode, groupId, idleTime);

            // Notify pool to undeploy this worker
            String address = "pool." + poolCode + ".workerIdle";
            vertx.eventBus().send(address, new WorkerIdle(groupId, deploymentID()));
        }
    }

    // === Message types for worker lifecycle ===

    public record WorkerIdle(String groupId, String deploymentId) {}
    public record BatchGroupFailed(String batchId, String groupId) {}
}
