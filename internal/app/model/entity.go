package model

import (
    "time"
)

type Entity struct {
    ID        uint      `gorm:"primaryKey"`
    Name      string    `gorm:"not null"`
    Type      string    `gorm:"not null"` // "onsen", "restaurant", "brand"
    CreatedAt time.Time
    UpdatedAt time.Time
    Topics    []EntityTopic `gorm:"foreignKey:EntityID"`
}

type EntityTopic struct {
    ID        uint      `gorm:"primaryKey"`
    EntityID  uint      `gorm:"not null;index"`
    Topic     string    `gorm:"not null"`
    CreatedAt time.Time
    UpdatedAt time.Time
    Trends    []TopicTrend `gorm:"foreignKey:TopicID"`
}

type TopicTrend struct {
    ID        uint      `gorm:"primaryKey"`
    TopicID   uint      `gorm:"not null;index"`
    Week      time.Time `gorm:"not null"`
    Score     float64   `gorm:"not null"`
    CreatedAt time.Time
    UpdatedAt time.Time
}
