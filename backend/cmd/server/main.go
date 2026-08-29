package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

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

	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/health", healthHandler)
	r.GET("/api/v1/parts", listPartsHandler)

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