FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . ./
RUN go build -o /out/api ./cmd/app
RUN go build -o /out/migrate ./cmd/migrate
RUN go build -o /out/atproto-revoke ./cmd/atproto-revoke
RUN go build -o /out/email-deliver ./cmd/email-deliver
RUN go build -o /out/identity-rekey ./cmd/identity-rekey
RUN go build -o /out/atproto-project ./cmd/atproto-project
RUN go build -o /out/backline-handoff ./cmd/backline-handoff

FROM alpine:3.22
WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=build /out/migrate /app/migrate
COPY --from=build /out/atproto-revoke /app/atproto-revoke
COPY --from=build /out/email-deliver /app/email-deliver
COPY --from=build /out/identity-rekey /app/identity-rekey
COPY --from=build /out/atproto-project /app/atproto-project
COPY --from=build /out/backline-handoff /app/backline-handoff
EXPOSE 8080
CMD ["/app/api"]
