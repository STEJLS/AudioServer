package main

import (
	"time"

	"gopkg.in/mgo.v2/bson"
)

// IMetadata - интерфейс, который описывает поведение типов, которые возвращают метадынные
// Они должны уметь отдавать назвение песни, имя испольнителя, название жанра, битрейт, продолжительность песни
type IMetadata interface {
	GetTitle() string
	GetArtist() string
	GetGenre() string
	GetBitrate() int
	GetDuration() int
}

// SongInfo - структура, описывающая информацию песни. Хранится в БД.
type SongInfo struct {
	ID              bson.ObjectId `json:"id" bson:"_id,omitempty"`                // ID записи в БД
	FileName        string        `json:"FileName" bson:"FileName"`               // название песни
	Title           string        `json:"Title" bson:"Title"`                     // название песни
	Artist          string        `json:"Artist" bson:"Artist"`                   // исполнитель
	Genre           string        `json:"Genre" bson:"Genre"`                     // жанр
	Bitrate         int           `json:"Bitrate" bson:"Bitrate"`                 // килобит в секунду
	Duration        int           `json:"Duration" bson:"Duration"`               // продолжительность песни в секундах
	CountOfDownload int64         `json:"CountOfDownload" bson:"CountOfDownload"` // количество загрузок
	Size            int           `json:"Size" bson:"Size"`                       // размер в байтах
	UploadDate      time.Time     `json:"UploadDate" bson:"UploadDate"`           // дата загрузки
}

// NewSongInfo - конструктор для типа SongInfo на вход принимает id объекта БД, имя файла, размер файла и объект IMetadata
func NewSongInfo(id bson.ObjectId, fileName string, filesize int, metaData IMetadata) *SongInfo {
	return &SongInfo{id,
		fileName,
		metaData.GetTitle(),
		metaData.GetArtist(),
		metaData.GetGenre(),
		metaData.GetBitrate(),
		metaData.GetDuration(),
		initialCountOfDownloads,
		filesize,
		time.Now().UTC(),
	}
}

type PlayList struct {
	ID   bson.ObjectId `json:"id" bson:"_id,omitempty"` // ID записи в БД
	Name string        `json:"name" bson:"name"`        // название плэй листа
	IDs  []string      `json:"ids" bson:"ids"`          // список id песен
}
