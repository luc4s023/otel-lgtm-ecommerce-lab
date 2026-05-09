FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /out/frontend .

FROM alpine:3.20
RUN adduser -D app
USER app
COPY --from=build /out/frontend /frontend
EXPOSE 8080
ENTRYPOINT ["/frontend"]
