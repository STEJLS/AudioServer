package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"

	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// InitFlags - инициализирует флаги командной строки
func InitFlags() {
	flag.StringVar(&logSource, "log_source", "log.txt", "Source for log file")
	flag.StringVar(&configSource, "config_source", "config.xml", "Source for config file")
	flag.Parse()
}

// InitLogger - настраивает логгер, destination - файл куда писать логи
func InitLogger(destination string) *os.File {
	logfile, err := os.OpenFile(logSource, os.O_APPEND|os.O_CREATE, 0666)
	if err != nil {
		log.Fatalln("Ошибка. Файл логов (%q) не открылся: ", logSource, err)
	}

	log.SetOutput(logfile)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	return logfile
}

// connectToDB - устанавливет соединение с БД и инициализирует глобальные переменные
func connectToDB(host string, port int, DBName string) {
	var err error
	audioDBsession, err = mgo.Dial(fmt.Sprintf("mongodb://%v:%v", host, port))
	if err != nil {
		log.Fatalln(fmt.Sprintf("Фатал. При подключении к серверу БД(%v:%v): ", host, port) + err.Error())
	}
	songsColl = audioDBsession.DB(DBName).C("Songs")
	log.Printf("Инфо. Подключение к базе данных установлено.")
}

// saveFile - сохраняет копию обрабатываемой песни на диске под именем newFileName,
// если все прошло успешно, то возвращает nil, в противном случае объект ошибки
func saveFile(readSeeker io.ReadSeeker, newFileName string) error {
	_, err := readSeeker.Seek(0, os.SEEK_SET)
	if err != nil {
		log.Println("Ошибка. Не удалось перейти на начало копируемого файла: " + err.Error())
		return err
	}
	data, err := ioutil.ReadAll(readSeeker)
	if err != nil {
		log.Println("Ошибка. Не удалось прочитать файл пользователя: " + err.Error())
		return err
	}

	err = ioutil.WriteFile(newFileName, data, 0666)
	if err != nil {
		log.Println("Ошибка. Не удалось создать новый файл: " + err.Error())
		return err
	}

	log.Println("Инфо. Файл сохранен на диске")
	return nil
}

// removeFile - удаляет файл и обрабатывает возможные ошибки
func removeFile(fileName string) {
	err := os.Remove(fileName)
	if err != nil {
		log.Println("Ошибка. При удалении файла: " + err.Error())
	}
}

// CheckExistMetaInDB - проверяет на существование в БД переданных метаданных
// если такие данные есть, то возвращает true , инача false
func CheckExistMetaInDB(mataData IMetadata) (bool, error) {
	n, err := songsColl.Find(bson.M{"Title": mataData.GetTitle(),
		"Artist":   mataData.GetArtist(),
		"Genre":    mataData.GetGenre(),
		"Bitrate":  mataData.GetBitrate(),
		"Duration": mataData.GetDuration(),
	}).Count()

	if err != nil {
		log.Println("Ошибка. При поиске записи в БД: " + err.Error())
		return false, err
	}

	if n == 0 {
		return false, nil
	}

	return true, nil
}

// getCountOfMetadata - пытается извлечь переменную с именем count и возвращает его если оно корректно,
// в противном случае возвращается значение по умолчанию
func getCountOfMetadata(r *http.Request) int {
	var count int

	strCount := r.FormValue("count")

	if strCount == "" {
		log.Printf("Инфо. Количетсво запрашиваемых записей не указано, отдаю стандартное кол-во: %v", defaultCountMatadataForUpload)
		count = defaultCountMatadataForUpload
	} else {
		var err error
		count, err = strconv.Atoi(strCount)

		if err != nil {
			log.Println("Ошибка. Не получилось преобрахзовать введенное кол-во в число: " + err.Error())
			count = defaultCountMatadataForUpload
		}
	}

	return count
}
