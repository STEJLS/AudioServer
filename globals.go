package main

import mgo "gopkg.in/mgo.v2"

// logFileName - имя файла для логов, задается через флаг командной строки
var logSource string

// ConfigSource - имя файла для конфига, задается через флаг командной строки
var configSource string

// AudioDBsession - указаетль на сессию подключения к БД Audio
var audioDBsession *mgo.Session

// SongsColl - это указатель на подключение к коллекции Songs базы данных Audio
var songsColl *mgo.Collection

const (
	formFileName                  string = "file"      // имя файла в форме на сайте
	storageDirectory              string = "../music/" // место для хранения песен
	initialCountOfDownloads       int64  = 0           // начальное  количесвто скачиваний
	defaultCountMatadataForUpload int    = 25          // кол-во по умолчанию сколько метаданных будет отдаваться
)
