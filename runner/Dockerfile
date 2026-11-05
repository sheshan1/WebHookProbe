FROM golang:1.26-alpine AS build
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /webhookprobe ./cmd/webhookprobe
FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=build /webhookprobe /usr/local/bin/webhookprobe
USER 65532:65532
EXPOSE 8081
ENTRYPOINT ["webhookprobe"]
CMD ["serve","-listen","0.0.0.0:8081"]
