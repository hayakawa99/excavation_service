package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time" // timeパッケージを追加

	_ "github.com/lib/pq" // PostgreSQLドライバ
)

func ConnectDatabase() (*sql.DB, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable not set")
	}

	var db *sql.DB
	var err error
	maxRetries := 10 // 最大リトライ回数
	retryInterval := 5 * time.Second // リトライ間隔

	// データベースに接続（リトライ付き）
	for i := 0; i < maxRetries; i++ {
		log.Printf("Attempting to connect to database (Attempt %d/%d)...", i+1, maxRetries)
		db, err = sql.Open("postgres", databaseURL)
		if err != nil {
			log.Printf("Failed to open database connection: %v. Retrying in %s...", err, retryInterval)
			time.Sleep(retryInterval)
			continue // 次のリトライへ
		}

		// 接続の確認（Ping）
		if err = db.Ping(); err != nil {
			db.Close() // Pingに失敗したら接続を閉じる
			log.Printf("Failed to ping database: %v. Retrying in %s...", err, retryInterval)
			time.Sleep(retryInterval)
			continue // 次のリトライへ
		}

		// 接続成功
		fmt.Println("Successfully connected to database!")
		return db, nil
	}

	// 最大リトライ回数を超えても接続できなかった場合
	return nil, fmt.Errorf("failed to connect to database after %d retries", maxRetries)
}