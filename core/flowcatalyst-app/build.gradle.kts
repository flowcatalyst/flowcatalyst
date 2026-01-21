plugins {
    java
    id("io.quarkus")
}

repositories {
    mavenCentral()
    mavenLocal()
}

val quarkusPlatformGroupId: String by project
val quarkusPlatformArtifactId: String by project
val quarkusPlatformVersion: String by project

// ==========================================================================
// Platform UI Build (Vue.js)
// ==========================================================================
val uiDir = rootProject.file("packages/platform-ui-vue")
val uiDistDir = uiDir.resolve("dist")
val uiOutputDir = layout.buildDirectory.dir("resources/main/META-INF/resources")

val npmInstall by tasks.registering(Exec::class) {
    description = "Install npm dependencies for platform-ui-vue"
    group = "build"
    workingDir = uiDir
    commandLine("npm", "install", "--legacy-peer-deps")
    inputs.file(uiDir.resolve("package.json"))
    outputs.dir(uiDir.resolve("node_modules"))
}

// Build UI using existing generated SDK (no regeneration - avoids circular dependency)
val buildUi by tasks.registering(Exec::class) {
    description = "Build platform-ui-vue for production (uses existing SDK)"
    group = "build"
    dependsOn(npmInstall)
    workingDir = uiDir
    // Use vite build directly - skip api:generate to avoid circular dependency
    commandLine("npx", "vite", "build")
    // Note: Don't declare inputs/outputs - they conflict with same tasks in other modules
}

// ==========================================================================
// OpenAPI SDK Generation (run separately, not part of normal build)
// ==========================================================================
// To update the SDK after API changes:
//   ./gradlew :core:flowcatalyst-app:updateApiSdk
// This builds the app, extracts OpenAPI spec, and regenerates the TypeScript SDK

val copyOpenApiToFrontend by tasks.registering(Copy::class) {
    description = "Copy generated OpenAPI spec to frontend"
    group = "openapi"
    dependsOn("quarkusBuild")
    // OpenAPI is generated in project dir (not build dir) by quarkus.smallrye-openapi.store-schema-directory
    from(layout.projectDirectory.dir("openapi"))
    into(uiDir.resolve("openapi"))
    doFirst {
        println("Copying OpenAPI spec to frontend...")
    }
}

val generateApiSdk by tasks.registering(Exec::class) {
    description = "Generate TypeScript SDK from OpenAPI spec"
    group = "openapi"
    dependsOn(npmInstall, copyOpenApiToFrontend)
    workingDir = uiDir
    commandLine("npm", "run", "api:generate")
    // Note: Don't declare outputs.dir() here - it conflicts with buildUi's inputs.dir() in other modules
}

val updateApiSdk by tasks.registering {
    description = "Build app, extract OpenAPI, and regenerate TypeScript SDK"
    group = "openapi"
    dependsOn(generateApiSdk)
    doLast {
        println("API SDK updated! Now rebuild the UI or run a full build.")
    }
}

val copyUiToResources by tasks.registering(Copy::class) {
    description = "Copy built UI to META-INF/resources"
    group = "build"
    dependsOn(buildUi)
    from(uiDistDir)
    into(uiOutputDir)
}

tasks.named("processResources") {
    dependsOn(copyUiToResources)
}

dependencies {
    implementation(enforcedPlatform("${quarkusPlatformGroupId}:${quarkusPlatformArtifactId}:${quarkusPlatformVersion}"))

    // Core Quarkus (needed for startup/shutdown hooks)
    implementation("io.quarkus:quarkus-arc")

    // All modules - disable individually via config if not needed
    implementation(project(":core:flowcatalyst-platform"))
    implementation(project(":core:flowcatalyst-stream-processor"))
    implementation(project(":core:flowcatalyst-dispatch-scheduler"))

    // Queue client optional dependencies (needed at runtime for native builds)
    implementation("io.nats:jnats:2.24.1")
    implementation("org.xerial:sqlite-jdbc:3.47.1.0")

    // Vert.x for SPA fallback routing
    implementation("io.quarkus:quarkus-vertx-http")
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

// Uber-jar: ignore duplicate metadata files from dependencies
tasks.withType<Test> {
    useJUnitPlatform()
}
