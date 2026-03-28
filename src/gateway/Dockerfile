# ACE Platform — API Gateway (Java/Kotlin)
# Build: docker build -t ace-gateway ./src/gateway

FROM eclipse-temurin:21-jdk AS builder
WORKDIR /app
COPY . .
RUN ./gradlew bootJar --no-daemon

FROM eclipse-temurin:21-jre
COPY --from=builder /app/build/libs/*.jar /app.jar
EXPOSE 8080 9878
CMD ["java", "-jar", "/app.jar"]
