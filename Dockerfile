FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG APP=api
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./apps/${APP}

FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=build /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
