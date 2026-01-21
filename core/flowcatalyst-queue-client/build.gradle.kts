plugins {
    `java-library`
    id("io.quarkus")
}

repositories {
    mavenCentral()
    mavenLocal()
}

val quarkusPlatformGroupId: String by project
val quarkusPlatformArtifactId: String by project
val quarkusPlatformVersion: String by project

dependencies {
    implementation(enforcedPlatform("${quarkusPlatformGroupId}:${quarkusPlatformArtifactId}:${quarkusPlatformVersion}"))

    // Core Quarkus
    api("io.quarkus:quarkus-arc")

    // SQS support (optional at runtime)
    compileOnly("io.quarkiverse.amazonservices:quarkus-amazon-sqs:2.18.1")

    // ActiveMQ Artemis JMS support (optional at runtime, can be added later)
    // compileOnly("io.quarkiverse.artemis:quarkus-artemis-jms:3.4.0")

    // NATS JetStream support (optional at runtime)
    compileOnly("io.nats:jnats:2.24.1")

    // Embedded queue support - SQLite
    compileOnly("org.xerial:sqlite-jdbc:3.47.1.0")

    // Testing
    testImplementation("io.quarkus:quarkus-junit5")
    testImplementation("io.quarkus:quarkus-junit5-mockito")
}

group = "tech.flowcatalyst"
version = "1.0.0-SNAPSHOT"

java {
    sourceCompatibility = JavaVersion.VERSION_21
    targetCompatibility = JavaVersion.VERSION_21
}

tasks.withType<JavaCompile> {
    options.encoding = "UTF-8"
    options.compilerArgs.add("-parameters")
}

tasks.withType<Test> {
    useJUnitPlatform()
    systemProperty("java.util.logging.manager", "org.jboss.logmanager.LogManager")
}
