package main

import (
	"fmt"
	"log"

	"excavation_service/internal/app/db" // あなたのモジュール名/internal/app/db になっているか確認
)

func main() {
	fmt.Println("Application starting...")

	// データベースに接続
	dbConn, err := db.ConnectDatabase()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbConn.Close() // アプリケーション終了時に接続を閉じる

	// ここにアプリケーションの他の処理を記述...
	// Echoサーバーの起動など は次のステップで行います

	fmt.Println("Application started successfully.")

	// 簡単な待機（Ctrl+Cで終了）
	select {}
}