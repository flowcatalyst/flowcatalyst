// Root build file for multi-module project
plugins {
    java
    id("io.quarkus") apply false
}

allprojects {
    group = "tech.flowcatalyst"
    version = "1.0.0-SNAPSHOT"

    repositories {
        mavenCentral()
        mavenLocal()
    }
}

subprojects {
    apply(plugin = "java")

    java {
        sourceCompatibility = JavaVersion.VERSION_21
        targetCompatibility = JavaVersion.VERSION_21
    }

    tasks.withType<JavaCompile> {
        options.encoding = "UTF-8"
        options.compilerArgs.add("-parameters")
    }
}

// ==========================================================================
// OpenAPI SDK Generation Task
// ==========================================================================
// Regenerates the TypeScript SDK from the OpenAPI spec.
// Run this after changing backend API endpoints.
// The normal build uses existing generated SDK to avoid circular dependencies.
tasks.register("updateApiSdk") {
    description = "Build app, extract OpenAPI spec, and regenerate TypeScript SDK"
    group = "openapi"

    dependsOn(":core:flowcatalyst-app:updateApiSdk")

    doLast {
        println("")
        println("========================================")
        println("API SDK updated successfully!")
        println("========================================")
        println("The TypeScript SDK has been regenerated from the OpenAPI spec.")
        println("You can now run a full build to include the updated SDK.")
        println("========================================")
    }
}
