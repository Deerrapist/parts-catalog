package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

// Part — запись каталога в полной выгрузке (подход №1).
type Part struct {
	ID          string  `json:"id"`
	CategoryID  *string `json:"category_id"`
	Name        string  `json:"name"`
	Article     string  `json:"article"`
	Size        *string `json:"size"`
	Material    *string `json:"material"`
	WeightG     *int    `json:"weight_g"`
	Description *string `json:"description"`
	Version     int64   `json:"version"`
}

// DeltaPart — запись в дельте (подход №2). Все поля кроме id и version
// опциональны: для удалённой записи передаётся только идентификатор.
type DeltaPart struct {
	ID          string  `json:"id"`
	CategoryID  *string `json:"category_id,omitempty"`
	Name        *string `json:"name,omitempty"`
	Article     *string `json:"article,omitempty"`
	Size        *string `json:"size,omitempty"`
	Material    *string `json:"material,omitempty"`
	WeightG     *int    `json:"weight_g,omitempty"`
	Description *string `json:"description,omitempty"`
	Version     int64   `json:"version"`
	Deleted     bool    `json:"deleted,omitempty"`
}

var db *sql.DB

func main() {
	dsn := fmt.Sprintf(
		"host=%s port=%s dbname=%s user=%s password=%s sslmode=disable",
		os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_NAME"),
		os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"),
	)

	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	// sql.Open не подключается, только готовит пул. Проверяем связь явно.
	for i := 0; i < 10; i++ {
		if err = db.Ping(); err == nil {
			break
		}
		log.Printf("ping failed (%d/10): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("database unreachable: %v", err)
	}
	log.Println("database connected")

	// gin.New() вместо gin.Default(): без Logger-middleware,
	// чтобы пер-запросный вывод в stdout не входил в измеряемое время.
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", healthHandler)

	// Ветка 1: наивная полная выгрузка.
	r.GET("/api/v1/parts", listPartsHandler)
	// Ветка 3: дельта по версии.
	r.GET("/api/v1/parts/delta", deltaPartsHandler)

	// Ветки 2 и 4: то же со сжатием.
	gz := r.Group("/api/v1/gz")
	gz.Use(gzip.Gzip(gzip.DefaultCompression))
	gz.GET("/parts", listPartsHandler)
	gz.GET("/parts/delta", deltaPartsHandler)

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("listening on :%s", port)
	if err := r.Run("0.0.0.0:" + port); err != nil {
		log.Fatal(err)
	}
}

func healthHandler(c *gin.Context) {
	if err := db.Ping(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db unreachable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Подход №1: полная выгрузка каталога. База сравнения для дельты.
// Удалённые записи не отдаются — клиент получает актуальный срез.
func listPartsHandler(c *gin.Context) {
	rows, err := db.Query(`
		SELECT id, category_id, name, article, size, material,
		       weight_g, description, version
		FROM parts
		WHERE deleted_at IS NULL
		ORDER BY version`)
	if err != nil {
		log.Printf("query: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}
	defer rows.Close()

	parts := make([]Part, 0)
	for rows.Next() {
		var p Part
		if err := rows.Scan(&p.ID, &p.CategoryID, &p.Name, &p.Article,
			&p.Size, &p.Material, &p.WeightG, &p.Description, &p.Version); err != nil {
			log.Printf("scan: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan failed"})
			return
		}
		parts = append(parts, p)
	}
	if err := rows.Err(); err != nil {
		log.Printf("rows: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "iteration failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"parts": parts, "count": len(parts)})
}

// Подход №2: дельта-синхронизация по монотонному счётчику версий.
// В отличие от полной выгрузки, удалённые записи ОТДАЮТСЯ с флагом
// deleted — иначе офлайн-клиент не узнает об удалении.
func deltaPartsHandler(c *gin.Context) {
	since, err := strconv.ParseInt(c.DefaultQuery("since_version", "0"), 10, 64)
	if err != nil || since < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid since_version"})
		return
	}

	rows, err := db.Query(`
		SELECT id, category_id, name, article, size, material,
		       weight_g, description, version,
		       (deleted_at IS NOT NULL) AS deleted
		FROM parts
		WHERE version > $1
		ORDER BY version`, since)
	if err != nil {
		log.Printf("delta query: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}
	defer rows.Close()

	parts := make([]DeltaPart, 0)
	maxVersion := since

	for rows.Next() {
		var p DeltaPart
		if err := rows.Scan(&p.ID, &p.CategoryID, &p.Name, &p.Article,
			&p.Size, &p.Material, &p.WeightG, &p.Description,
			&p.Version, &p.Deleted); err != nil {
			log.Printf("delta scan: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan failed"})
			return
		}
		// Для удалённой записи содержимое полей не нужно:
		// клиент удаляет запись локально по идентификатору.
		if p.Deleted {
			p = DeltaPart{ID: p.ID, Version: p.Version, Deleted: true}
		}
		if p.Version > maxVersion {
			maxVersion = p.Version
		}
		parts = append(parts, p)
	}
	if err := rows.Err(); err != nil {
		log.Printf("delta rows: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "iteration failed"})
		return
	}

	// version — новый курсор клиента. При пустой дельте равен since,
	// иначе клиенту нечего сохранить и он застрянет на старом значении.
	c.JSON(http.StatusOK, gin.H{
		"parts":   parts,
		"count":   len(parts),
		"version": maxVersion,
	})
}