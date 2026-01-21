import java.io.ByteArrayOutputStream

plugins {
    java
    id("io.quarkus")
}

// ==========================================================================
// jlink Custom JRE Configuration
// ==========================================================================

val jlinkOutputDir = layout.buildDirectory.dir("jlink-jre")
val jlinkDistDir = layout.buildDirectory.dir("jlink-dist")
val javaHome = System.getProperty("java.home")

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
// OpenAPI SDK Generation
// ==========================================================================
// To update the SDK after API changes, use the root or app module task:
//   ./gradlew :updateApiSdk
// The SDK is shared between app and dev-build modules.

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

// ==========================================================================
// Exclude heavy dependencies not needed for dev build (~60MB savings)
// Note: aws-crt cannot be excluded when building native images (GraalVM needs it for analysis)
// ==========================================================================
val isNativeBuild = project.hasProperty("quarkus.native.enabled") ||
    System.getProperty("quarkus.native.enabled") == "true"

configurations.all {
    // Note: mongodb-crypt is optional - driver works without it, just no CSFLE support
    // We DON'T exclude it here - GraalVM needs the class references to build
    if (!isNativeBuild) {
        exclude(group = "software.amazon.awssdk.crt", module = "aws-crt")  // 18MB - using URL client instead
    }
    exclude(group = "org.hibernate.orm", module = "hibernate-core")    // 13MB - not using JPA/Hibernate
    exclude(group = "net.bytebuddy", module = "byte-buddy")            // 8.6MB - runtime code gen
}

dependencies {
    // Quarkus BOM
    implementation(enforcedPlatform("${quarkusPlatformGroupId}:${quarkusPlatformArtifactId}:${quarkusPlatformVersion}"))

    // ==========================================================================
    // Core Quarkus
    // ==========================================================================
    implementation("io.quarkus:quarkus-arc")

    // ==========================================================================
    // All FlowCatalyst Modules
    // ==========================================================================
    implementation(project(":core:flowcatalyst-platform"))
    implementation(project(":core:flowcatalyst-message-router"))
    implementation(project(":core:flowcatalyst-stream-processor"))
    implementation(project(":core:flowcatalyst-dispatch-scheduler"))
    implementation(project(":core:flowcatalyst-outbox-processor"))

    // ==========================================================================
    // Database Drivers for Outbox Processor Options
    // ==========================================================================
    implementation("io.quarkus:quarkus-jdbc-postgresql")
    implementation("io.quarkus:quarkus-jdbc-mysql")
    implementation("io.quarkus:quarkus-agroal")

    // ==========================================================================
    // GraalVM Native Image Support
    // mongodb-crypt needed for native-image class analysis (MongoCrypts references)
    // Not used at runtime - CSFLE is not enabled
    // ==========================================================================
    compileOnly("org.mongodb:mongodb-crypt:1.12.0")

    // ==========================================================================
    // Testing
    // ==========================================================================
    testImplementation("io.quarkus:quarkus-junit5")
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

// ==========================================================================
// Java 24+ Compatibility
// ==========================================================================
tasks.withType<JavaExec> {
    jvmArgs("--add-opens", "java.base/java.lang=ALL-UNNAMED")
}

tasks.withType<Test> {
    jvmArgs("--add-opens", "java.base/java.lang=ALL-UNNAMED")
    useJUnitPlatform()
}

// ==========================================================================
// jlink Build Tasks
// ==========================================================================

val quarkusBuildUberJar by tasks.registering(Exec::class) {
    description = "Build Quarkus uber-jar for jlink packaging"
    group = "jlink"

    workingDir = rootProject.projectDir

    commandLine(
        "./gradlew",
        ":core:flowcatalyst-dev-build:quarkusBuild",
        "-Dquarkus.package.jar.type=uber-jar",
        "--no-daemon"
    )

    val uberJar = layout.buildDirectory.file("flowcatalyst-dev-build-1.0.0-SNAPSHOT-runner.jar")
    outputs.file(uberJar)
}

val jlinkAnalyzeModules by tasks.registering(Exec::class) {
    description = "Analyze uber-jar to determine required Java modules"
    group = "jlink"
    dependsOn(quarkusBuildUberJar)

    val uberJar = layout.buildDirectory.file("flowcatalyst-dev-build-1.0.0-SNAPSHOT-runner.jar")
    val modulesFile = layout.buildDirectory.file("jlink-modules.txt")

    inputs.file(uberJar)
    outputs.file(modulesFile)

    workingDir = layout.buildDirectory.get().asFile

    commandLine(
        "$javaHome/bin/jdeps",
        "--ignore-missing-deps",
        "--print-module-deps",
        "--multi-release", "21",
        uberJar.get().asFile.absolutePath
    )

    doFirst {
        if (!uberJar.get().asFile.exists()) {
            throw GradleException("Uber-jar not found at ${uberJar.get().asFile.absolutePath}. Run quarkusBuild with uber-jar type first.")
        }
    }

    standardOutput = ByteArrayOutputStream()

    doLast {
        val modules = (standardOutput as ByteArrayOutputStream).toString().trim()
        modulesFile.get().asFile.writeText(modules)
        println("Detected modules: $modules")
    }
}

val jlinkCreateJre by tasks.registering(Exec::class) {
    description = "Create custom JRE using jlink"
    group = "jlink"
    dependsOn(jlinkAnalyzeModules)

    val modulesFile = layout.buildDirectory.file("jlink-modules.txt")
    val jreDir = jlinkOutputDir.get().asFile

    inputs.file(modulesFile)
    outputs.dir(jreDir)

    doFirst {
        // Clean previous jlink output
        if (jreDir.exists()) {
            jreDir.deleteRecursively()
        }

        // Read detected modules and add essential ones
        val detectedModules = if (modulesFile.get().asFile.exists()) {
            modulesFile.get().asFile.readText().trim()
        } else {
            ""
        }

        // Essential modules for Quarkus/Netty/MongoDB
        val essentialModules = listOf(
            "java.base",
            "java.logging",
            "java.naming",
            "java.sql",
            "java.xml",
            "java.desktop",        // For AWT/image processing
            "java.management",
            "java.security.jgss",
            "java.instrument",
            "jdk.unsupported",     // For Netty/Unsafe access
            "jdk.crypto.ec",       // For TLS/HTTPS
            "jdk.zipfs",           // For JAR handling
            "jdk.management",
            "java.net.http"
        )

        val allModules = (detectedModules.split(",").filter { it.isNotBlank() } + essentialModules)
            .distinct()
            .sorted()
            .joinToString(",")

        println("Creating custom JRE with modules: $allModules")

        commandLine(
            "$javaHome/bin/jlink",
            "--add-modules", allModules,
            "--output", jreDir.absolutePath,
            "--strip-debug",
            "--compress", "zip-6",
            "--no-header-files",
            "--no-man-pages"
        )
    }

    // Placeholder - actual command set in doFirst
    commandLine("echo", "jlink")

    isIgnoreExitValue = false
}

val jlinkPackage by tasks.registering(Copy::class) {
    description = "Package uber-jar with custom JRE into distribution"
    group = "jlink"
    dependsOn(jlinkCreateJre, quarkusBuildUberJar)

    val distDir = jlinkDistDir.get().asFile
    val uberJar = layout.buildDirectory.file("flowcatalyst-dev-build-1.0.0-SNAPSHOT-runner.jar")

    doFirst {
        if (distDir.exists()) {
            distDir.deleteRecursively()
        }
    }

    // Copy custom JRE
    from(jlinkOutputDir) {
        into("jre")
    }

    // Copy uber-jar
    from(uberJar) {
        rename { "flowcatalyst-dev.jar" }
    }

    into(distDir)

    doLast {
        // Create launcher scripts
        val binDir = File(distDir, "bin")
        binDir.mkdirs()

        // Unix launcher
        val unixLauncher = File(binDir, "flowcatalyst-dev")
        unixLauncher.writeText("""
            #!/bin/bash
            SCRIPT_DIR="$(cd "$(dirname "${'$'}0")" && pwd)"
            DIST_DIR="$(dirname "${'$'}SCRIPT_DIR")"

            exec "${'$'}DIST_DIR/jre/bin/java" \
                --add-opens java.base/java.lang=ALL-UNNAMED \
                -jar "${'$'}DIST_DIR/flowcatalyst-dev.jar" \
                "${'$'}@"
        """.trimIndent() + "\n")
        unixLauncher.setExecutable(true)

        // Windows launcher
        val windowsLauncher = File(binDir, "flowcatalyst-dev.bat")
        windowsLauncher.writeText("""
            @echo off
            set SCRIPT_DIR=%~dp0
            set DIST_DIR=%SCRIPT_DIR%..

            "%DIST_DIR%\jre\bin\java.exe" ^
                --add-opens java.base/java.lang=ALL-UNNAMED ^
                -jar "%DIST_DIR%\flowcatalyst-dev.jar" ^
                %*
        """.trimIndent() + "\r\n")

        println("")
        println("========================================")
        println("jlink distribution created successfully!")
        println("========================================")
        println("Location: ${distDir.absolutePath}")
        println("")
        println("Contents:")
        println("  - jre/          Custom Java runtime (~50-80MB)")
        println("  - flowcatalyst-dev.jar")
        println("  - bin/flowcatalyst-dev      (Unix launcher)")
        println("  - bin/flowcatalyst-dev.bat  (Windows launcher)")
        println("")
        println("To run: ./build/jlink-dist/bin/flowcatalyst-dev")
        println("========================================")
    }
}

val jlinkBuild by tasks.registering {
    description = "Full jlink build: uber-jar + custom JRE + distribution"
    group = "jlink"
    dependsOn(jlinkPackage)
}
