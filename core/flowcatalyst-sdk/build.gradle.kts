plugins {
    `java-library`
}

repositories {
    mavenCentral()
    mavenLocal()
}

val quarkusPlatformGroupId: String by project
val quarkusPlatformArtifactId: String by project
val quarkusPlatformVersion: String by project

dependencies {
    // Quarkus BOM for consistent versions
    implementation(enforcedPlatform("${quarkusPlatformGroupId}:${quarkusPlatformArtifactId}:${quarkusPlatformVersion}"))

    // ==========================================================================
    // Core Dependencies (for entities and utilities)
    // ==========================================================================

    // JPA/Hibernate for entity definitions
    api("io.quarkus:quarkus-hibernate-orm-panache")

    // Jackson for JSON handling
    api("com.fasterxml.jackson.core:jackson-databind")

    // Validation
    api("io.quarkus:quarkus-hibernate-validator")

    // ==========================================================================
    // REST Client (for sync service)
    // ==========================================================================
    implementation("io.quarkus:quarkus-rest-client-jackson")

    // ==========================================================================
    // Utilities
    // ==========================================================================

    // TSID for ID generation
    api("com.github.f4b6a3:tsid-creator:5.2.6")

    // ==========================================================================
    // Testing
    // ==========================================================================
    testImplementation("io.quarkus:quarkus-junit5")
    testImplementation("org.assertj:assertj-core:3.24.2")
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
