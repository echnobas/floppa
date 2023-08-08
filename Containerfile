FROM golang:1.20-alpine
WORKDIR /app
COPY go.mod ./
COPY go.sum ./
RUN go mod download
COPY *.go ./
RUN mkdir -p /release
RUN go build -o /floppa
WORKDIR /appdata
EXPOSE 8000
CMD [ "/floppa" ]