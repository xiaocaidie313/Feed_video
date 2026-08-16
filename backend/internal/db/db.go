package db

import (
	"feedvid/internal/account"
	"feedvid/internal/message"
	"feedvid/internal/social"
	"feedvid/internal/video"
	"feedvid/internal/worker"
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type Dbconfig struct {
	Host     string `yaml:"host"`
	User     string `yaml:"user"`
	Port     string `yaml:"port"`
	Password string `yaml:"password"`
	DbName   string `yaml:"dbname"`
}

func NewDB(config Dbconfig) (*gorm.DB, error) {
	//构造dsn  mysql 形式
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&Time=True&loc=Local", config.User, config.Password, config.Host, config.Port, config.DbName)

	// 连接数据库
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	//todo 可 switch case 一下 不同的数据库
	if err != nil {
		return nil, err
	}

	return db, nil
}

func CloseDB(db *gorm.DB) error {
	sqldb, err := db.DB() // 从gorm.DB中获取sql.DB  unwrap
	if err != nil {
		return err
	}
	return sqldb.Close()
}
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&account.Account{}, &video.Video{}, &video.Like{}, &video.Comment{},
		&social.Social{}, &video.OutboxMsg{}, &video.Tag{}, &video.VideoTag{},
		&message.Message{}, &worker.Notification{},
	)
}
