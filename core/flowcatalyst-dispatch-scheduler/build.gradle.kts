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

    // Shared modules
    implementation(project(":core:flowcatalyst-standby"))
    implementation(project(":core:flowcatalyst-queue-client"))
    implementation(project(":core:flowcatalyst-platform"))

    // Core Quarkus
    implementation("io.quarkus:quarkus-arc")
    implementation("io.quarkus:quarkus-scheduler")
    implementation("io.quarkus:quarkus-mongodb-panache")

    // JSON serialization
    implementation("io.quarkus:quarkus-jackson")

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
