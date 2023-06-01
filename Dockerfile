FROM golang:latest

WORKDIR /app

COPY . .

# Installs Go dependencies
RUN go mod download
 
# Builds your app with optional configuration
RUN go build -o /goserver

CMD ["/goserver"]
# CMD ["go","run","main.go"]