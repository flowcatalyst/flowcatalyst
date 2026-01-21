pluginManagement {
    val quarkusPluginVersion: String by settings
    repositories {
        mavenCentral()
        gradlePluginPortal()
        mavenLocal()
    }
    plugins {
        id("io.quarkus") version quarkusPluginVersion
    }
}

rootProject.name = "flowcatalyst"

// =============================================================================
// Core Modules (Java/Quarkus)
// =============================================================================

// Shared hot standby module (Redis-based leader election)
include("core:flowcatalyst-standby")

// Shared queue client abstraction (SQS, ActiveMQ, Embedded)
include("core:flowcatalyst-queue-client")

// Dispatch job scheduler (polls PENDING jobs, respects DispatchMode, queues to message router)
include("core:flowcatalyst-dispatch-scheduler")

// The main platform service (auth, admin, dispatch, mediation, event types)
include("core:flowcatalyst-platform")

// High-volume message pointer routing (scales independently)
include("core:flowcatalyst-message-router")

// Vert.x verticle-based message router (alternative implementation)
include("core:flowcatalyst-message-router-vertx")

// Stream processor (MongoDB change streams to projection collections)
include("core:flowcatalyst-stream-processor")

// SDK for integrating applications (events, roles)
include("core:flowcatalyst-sdk")

// Outbox processor (polls customer outbox tables, sends to FlowCatalyst)
include("core:flowcatalyst-outbox-processor")

// Benchmarks
include("core:flowcatalyst-benchmark")

// Developer build - single executable combining all services for local development
include("core:flowcatalyst-dev-build")

// =============================================================================
// Legacy Modules (TO BE REMOVED after migration verified)
// =============================================================================
include("core:flowcatalyst-router-app")
include("core:flowcatalyst-app")