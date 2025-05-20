package repository

import (
    "os"
    "testing"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
    "excavation_service/internal/app/model"
)

func setupTestDB() (*gorm.DB, error) {
    dsn := os.Getenv("TEST_DATABASE_URL")
    return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}

func TestEntityCRUD(t *testing.T) {
    db, err := setupTestDB()
    if err != nil {
        t.Fatalf("DB接続失敗: %v", err)
    }

    err = db.AutoMigrate(&model.Entity{})
    if err != nil {
        t.Fatalf("マイグレーション失敗: %v", err)
    }

    entity := model.Entity{Name: "テスト温泉", Type: "onsen"}
    if err := db.Create(&entity).Error; err != nil {
        t.Fatalf("登録失敗: %v", err)
    }

    var found model.Entity
    if err := db.First(&found, "id = ?", entity.ID).Error; err != nil {
        t.Fatalf("取得失敗: %v", err)
    }

    if found.Name != entity.Name {
        t.Fatalf("取得内容不一致: got %v, want %v", found.Name, entity.Name)
    }
}
