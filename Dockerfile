FROM golang:1.25-trixie AS build

WORKDIR /go/src/app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o llm-proxy ./cmd/llm-proxy

FROM gcr.io/distroless/static-debian12

COPY --from=build /go/src/app/llm-proxy /llm-proxy

ENTRYPOINT ["/llm-proxy"]
