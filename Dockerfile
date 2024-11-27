# Usa la imagen oficial de Go como base
FROM golang:latest

# Establece la zona horaria en Lima, Perú
ENV TZ=America/Lima

LABEL maintainer="Your Name <cjuarezr99@gmail.com>"

# Define los argumentos de construcción y las variables de entorno
ARG SMTP_SERVER
ARG SMTP_PORT
ARG EMAIL
ARG EMAILOUT
ARG PASSWORD
ARG DATABASE_URL


ENV SMTP_SERVER=$SMTP_SERVER
ENV SMTP_PORT=$SMTP_PORT
ENV EMAIL=$EMAIL
ENV EMAILOUT=$EMAILOUT
ENV PASSWORD=$PASSWORD
ENV DATABASE_URL=$DATABASE_URL

# Establece el directorio de trabajo en /usr/src/app
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copia el resto de la aplicación al directorio de trabajo
COPY . .

# Build the Go app
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Expose port 4610 to the outside
EXPOSE 4610

# Command to run the executable
CMD ["./main"]