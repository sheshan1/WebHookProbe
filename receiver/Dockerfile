FROM eclipse-temurin:25-jdk-alpine AS build
WORKDIR /build
COPY .mvn .mvn
COPY mvnw pom.xml checkstyle.xml .springjavaformatconfig ./
COPY src src
RUN --mount=type=cache,target=/root/.m2 chmod +x mvnw && ./mvnw -q -DskipTests package

FROM eclipse-temurin:25-jre-alpine
RUN apk add --no-cache curl
WORKDIR /app
COPY --from=build /build/target/receiver-0.0.1-SNAPSHOT.jar app.jar
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["java","-jar","app.jar"]
