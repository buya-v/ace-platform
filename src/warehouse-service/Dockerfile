# ACE Platform — Warehouse Service (Java/Kotlin)
# Build: docker build -t ace-warehouse-service ./src/warehouse-service

FROM eclipse-temurin:21-jdk AS builder
WORKDIR /app
COPY . .
RUN ./gradlew bootJar --no-daemon

FROM eclipse-temurin:21-jre
COPY --from=builder /app/build/libs/*.jar /app.jar
EXPOSE 8080
CMD ["java", "-jar", "/app.jar"]
