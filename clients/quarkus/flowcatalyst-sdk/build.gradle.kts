plugins {
    java
    `java-library`
    `maven-publish`
}

group = "tech.flowcatalyst"
version = "1.0.0-SNAPSHOT"

repositories {
    mavenCentral()
}

val quarkusVersion = "3.17.5"

dependencies {
    // Quarkus BOM
    api(platform("io.quarkus.platform:quarkus-bom:${quarkusVersion}"))

    // REST Client
    api("io.quarkus:quarkus-rest-client-jackson")

    // OIDC Client for authentication
    api("io.quarkus:quarkus-oidc-client")

    // CDI
    api("io.quarkus:quarkus-arc")

    // JSON processing
    api("io.quarkus:quarkus-jackson")

    // MongoDB Panache (for outbox)
    compileOnly("io.quarkus:quarkus-mongodb-panache")

    // Validation
    api("io.quarkus:quarkus-hibernate-validator")

    // TSID generation
    api("com.github.f4b6a3:tsid-creator:5.2.6")

    // Lombok
    compileOnly("org.projectlombok:lombok:1.18.36")
    annotationProcessor("org.projectlombok:lombok:1.18.36")

    // Testing
    testImplementation("io.quarkus:quarkus-junit5")
    testImplementation("io.quarkus:quarkus-junit5-mockito")
    testImplementation("org.assertj:assertj-core:3.24.2")
}

java {
    sourceCompatibility = JavaVersion.VERSION_21
    targetCompatibility = JavaVersion.VERSION_21
    withSourcesJar()
    withJavadocJar()
}

tasks.withType<JavaCompile> {
    options.encoding = "UTF-8"
    options.compilerArgs.add("-parameters")
}

tasks.withType<Javadoc> {
    // Lombok-generated code causes javadoc issues
    // Exclude classes with Lombok builders from javadoc
    exclude("**/outbox/dto/**")
    options {
        (this as StandardJavadocDocletOptions).addStringOption("Xdoclint:none", "-quiet")
    }
}

publishing {
    publications {
        create<MavenPublication>("maven") {
            from(components["java"])

            pom {
                name.set("FlowCatalyst Quarkus SDK")
                description.set("Official Quarkus SDK for the FlowCatalyst Platform")
                url.set("https://github.com/flowcatalyst/flowcatalyst-quarkus-sdk")

                licenses {
                    license {
                        name.set("MIT License")
                        url.set("https://opensource.org/licenses/MIT")
                    }
                }
            }
        }
    }
}
