package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	_ "github.com/lib/pq"
	"github.com/nfnt/resize"
	gomail "gopkg.in/gomail.v2"
)

// Estructura para los datos del formulario
type PaymentForm struct {
	Nombres         string `json:"nombres"`
	Apellidos       string `json:"apellidos"`
	Correo          string `json:"correo"`
	Telefono        string `json:"telefono"`
	Universidad     string `json:"universidad"`
	Entrada         string `json:"entrada"`
	Codigo          string `json:"codigo"`
	Carrera         string `json:"carrera"`
	TipoOperacion   string `json:"tipo_operacion"`
	NumeroOperacion string `json:"numero_operacion"`
}

func main() {
	// Cargar las variables del entorno
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error al cargar el archivo .env")
	}

	// Obtén la URI desde la variable de entorno
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL no está configurada")
	}

	// Establecer la conexión con la base de datos usando GORM
	//db, err := sql.Open("postgres", "user=coneimera password=123456 dbname=coneimera sslmode=disable")
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("Error al abrir la conexión:", err)
	}
	// Verificar conexión
	if err := db.Ping(); err != nil {
		log.Fatal("Error al conectar con la base de datos:", err)
	}
	// Crear la tabla si no existe
	createTables(db)

	// Crear servidor web con Echo
	e := echo.New()

	// Configuración de CORS
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodPost},
	}))

	e.POST("/sendemail", func(c echo.Context) error {
		// Parsear los datos manualmente desde el formulario
		form := &PaymentForm{
			Nombres:         c.FormValue("nombres"),
			Apellidos:       c.FormValue("apellidos"),
			Correo:          c.FormValue("correo"),
			Telefono:        c.FormValue("telefono"),
			Universidad:     c.FormValue("universidad"),
			Entrada:         c.FormValue("entrada"),
			Codigo:          c.FormValue("codigo"),
			Carrera:         c.FormValue("carrera"),
			TipoOperacion:   c.FormValue("tipo_operacion"),
			NumeroOperacion: c.FormValue("numero_operacion"),
		}

		// Validar campos obligatorios
		missingFields := []string{}
		if form.Nombres == "" {
			missingFields = append(missingFields, "nombres")
		}
		if form.Apellidos == "" {
			missingFields = append(missingFields, "apellidos")
		}
		if form.Correo == "" || !isValidEmail(form.Correo) {
			missingFields = append(missingFields, "correo")
		}
		if form.Telefono == "" {
			missingFields = append(missingFields, "telefono")
		}
		if form.Universidad == "" {
			missingFields = append(missingFields, "universidad")
		}
		if form.Entrada == "" {
			missingFields = append(missingFields, "entrada")
		}
		if form.Carrera == "" {
			missingFields = append(missingFields, "carrera")
		}
		if form.TipoOperacion == "" {
			missingFields = append(missingFields, "tipo_operacion")
		}
		if form.NumeroOperacion == "" {
			missingFields = append(missingFields, "numero_operacion")
		}

		// Si faltan campos, devolver un error
		if len(missingFields) > 0 {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"error":          "Faltan campos obligatorios",
				"missing_fields": missingFields,
			})
		}

		// Validar que el número de operación no esté registrado
		var exists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM pagos WHERE numero_operacion = $1 AND tipo_operacion = $2)", form.NumeroOperacion, form.TipoOperacion).Scan(&exists)
		if err != nil {
			log.Printf("Error al ejecutar la consulta: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error en la base de datos"})
		}
		if exists {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Número de operación ya registrado previamente"})
		}

		// Validar el archivo `comprobante_pago`
		file, err := c.FormFile("comprobante_pago")
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Debe cargar un archivo de comprobante de pago válido"})
		}

		// Validar tamaño del archivo (5 MB = 5 * 1024 * 1024 bytes)
		if file.Size > 5*1024*1024 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "El archivo supera el tamaño máximo permitido de 5 MB"})
		}

		// Abrir el archivo
		src, err := file.Open()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error al abrir el archivo"})
		}
		defer src.Close()

		buffer := make([]byte, 512)
		if _, err := src.Read(buffer); err != nil && err != io.EOF {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error al leer el archivo"})
		}
		fileType := http.DetectContentType(buffer)
		src.Seek(0, 0)

		var fileData bytes.Buffer

		// Comprobar si es una imagen o un PDF
		if fileType == "image/jpeg" || fileType == "image/png" || fileType == "image/gif" || fileType == "image/bmp" || fileType == "image/tiff" {
			// Decodificar la imagen
			img, _, err := image.Decode(src)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Formato de imagen inválido"})
			}

			// Comprimir la imagen
			compressed := resize.Resize(800, 0, img, resize.Lanczos3)

			// Guardar la imagen en un formato estándar (JPEG o PNG)
			if err := jpeg.Encode(&fileData, compressed, nil); err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error al comprimir la imagen"})
			}
		} else if fileType == "application/pdf" {
			// Si el archivo es un PDF, copiar su contenido
			if _, err := io.Copy(&fileData, src); err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error al leer el archivo PDF"})
			}
		} else {
			// Si el tipo de archivo no es ni imagen ni PDF
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "El tipo de archivo no está permitido"})
		}

		// Guardar en la base de datos
		_, err = db.Exec(`
        INSERT INTO pagos (nombres, apellidos, correo, telefono, universidad, entrada, codigo, carrera, tipo_operacion, numero_operacion) 
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			form.Nombres, form.Apellidos, form.Correo, form.Telefono, form.Universidad, form.Entrada, form.Codigo, form.Carrera, form.TipoOperacion, form.NumeroOperacion,
		)
		if err != nil {
			log.Printf("Error al insertar en la base de datos: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error al insertar en la base de datos"})
		}
		// Enviar el correo
		m := gomail.NewMessage()
		m.SetHeader("From", os.Getenv("EMAIL"))
		m.SetHeader("To", os.Getenv("EMAILOUT"))
		m.SetHeader("Subject", "Nuevo Pago Registrado")

		body := formatBody(form)
		m.SetBody("text/html", body)
		m.Attach(file.Filename, gomail.SetCopyFunc(func(w io.Writer) error {
			_, err := w.Write(fileData.Bytes())
			return err
		}))

		port, _ := strconv.Atoi(os.Getenv("SMTP_PORT"))
		d := gomail.NewDialer(os.Getenv("SMTP_SERVER"), port, os.Getenv("EMAIL"), os.Getenv("PASSWORD"))
		d.SSL = true

		if err := d.DialAndSend(m); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error al enviar el correo"})
		}

		return c.JSON(http.StatusOK, map[string]string{"message": "Formulario enviado con éxito"})
	})

	// Iniciar el servidor
	e.Start(":4610")
}

// Función para crear las tablas si no existen
func createTables(db *sql.DB) {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS pagos (
			id SERIAL PRIMARY KEY,
			nombres VARCHAR(255),
			apellidos VARCHAR(255),
			correo VARCHAR(255),
			telefono VARCHAR(50),
			universidad VARCHAR(255),
			entrada VARCHAR(50),
			codigo VARCHAR(50),
			carrera VARCHAR(255),
			tipo_operacion VARCHAR(50),
			numero_operacion VARCHAR(50) UNIQUE
		);
	`)
	if err != nil {
		log.Fatal("Error al crear las tablas:", err)
	} else {
		fmt.Println("Tablas creadas o ya existen")
	}
}

// Formatear los datos del formulario para el cuerpo del correo
func formatBody(form *PaymentForm) string {
	return fmt.Sprintf(`
		Nombres: %s<br>
		Apellidos: %s<br>
		Correo: %s<br>
		Teléfono: %s<br>
		Universidad: %s<br>
		Entrada: %s<br>
		Código: %s<br>
		Carrera: %s<br>
		Tipo de Operación: %s<br>
		Número de Operación: %s<br>`,
		form.Nombres, form.Apellidos, form.Correo, form.Telefono, form.Universidad, form.Entrada, form.Codigo, form.Carrera, form.TipoOperacion, form.NumeroOperacion)
}

// Función para validar el correo electrónico
func isValidEmail(email string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return re.MatchString(email)
}
