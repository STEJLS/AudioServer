package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/STEJLS/AudioServer/XMLconfig"
	"github.com/STEJLS/AudioServer/mp3"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// logFileName - имя файла для логов, задается через флаг командной строки
var logSource string

// ConfigSource - имя файла для конфига, задается через флаг командной строки
var configSource string

// AudioDBsession - указаетль на сессию подключения к БД Audio
var audioDBsession *mgo.Session

// SongsColl - это указатель на подключение к коллекции Songs базы данных Audio
var songsColl *mgo.Collection

const (
	formFileName            string = "file"     // имя файла в форме на сайте
	storageDirectory        string = "./music/" // место для хранения песен
	initialCountOfDownloads int64  = 0          // начальное  количесвто скачиваний
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
		time.Now().Add(time.Hour * time.Duration(3)), //+3 часа - мск
	}
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

func main() {
	InitFlags()
	logFile := InitLogger(logSource)
	defer logFile.Close()

	config := XMLconfig.Get(configSource)

	connectToDB(config.Db.Host, config.Db.Port, config.Db.Name)
	defer audioDBsession.Close()

	server := http.Server{
		Addr: fmt.Sprintf(":%v", config.HTTP.Port),
		//Addr: fmt.Sprintf("%v:%v", config.HTTP.Host, config.HTTP.Port),
	}

	http.HandleFunc("/addSong", addSong)
	http.HandleFunc("/addSongForm", addSongForm)

	err := server.ListenAndServe()
	if err != nil {
		log.Println(err.Error())
	}
}

//////////////////////////////////////////////////////////////////////////////////////////
//////////////////////////////////////////////////////////////////////////////////////////
//////////////////////////////////////////////////////////////////////////////////////////
//////////////////////////////////////////////////////////////////////////////////////////

func addSongForm(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`<html>
		<head>
		<meta http-equiv="Content-Type" content="text/html; charset=utf-8" />
		<title>AudioServer</title>
		</head>
		<body>
		<form action="/addSong" 
		method="post" enctype="multipart/form-data">
		<input type="file" name="file">
		<input type="submit"/>
		</form>
		</body>
		</html>`),
	)
}

// addSong - добавляет песню в базу данных, копирует файл на диск в папку /music(имя песни - id записи из БД)
// Если файл добавлен ответ - "Файл успешно добавлен" - 200
// Если не получилось извлечь файл из формы ответ - "Файл не найден" - 400
// Если добавляемый файл уже есть в системе - "Загружаемый файл уже есть в системе" - 400
// Если тип файла не поддерижвается ответ - "Данный формат не поддерживается" - 415
// Если при создании копии на диске произошла ошибка ответ - "Неполадки на сервере, повторите попытку позже" - 500
// Если при записи метаданных в БД произошла ошибка ответ - "Неполадки на сервере, повторите попытку позже" - 500

// сделать проверку на вставку одинковых файлов
func addSong(w http.ResponseWriter, r *http.Request) {
	fd, fh, err := r.FormFile(formFileName)
	if err != nil {
		log.Printf("Ошибка. Добавление песни не удалось. При поиске в форме файла с именем %q: %v\n", formFileName, err.Error())
		http.Error(w, "Файл не найден", http.StatusBadRequest)
		return
	}
	log.Printf("Инфо. Файл %v поступил на обработку\n", fh.Filename)

	extension := filepath.Ext(fh.Filename)
	var metaData IMetadata

	switch extension {
	case ".mp3":
		metaData = mp3.ParseTags(fd)
		break
	case ".flac":
		// tag := flac.ParseTags(fd)
		break
	case ".ogg":
		// tag := ogg.ParseTags(fd)
		break
	}

	if metaData == nil {
		log.Println("Инфо. Данный формат не поддерживается: " + extension)
		http.Error(w, "Данный формат не поддерживается", http.StatusUnsupportedMediaType)
		return
	}
	log.Println("Инфо. Метаданные получены")

	flag, err := CheckExistMetaInDB(metaData)
	if err != nil {
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	if flag {
		log.Println("Инфо. Данный файл уже есть в системе")
		http.Error(w, "Данный файл уже есть в системе", http.StatusBadRequest)
		return
	}

	id := bson.NewObjectId()

	err = saveFile(fd, storageDirectory+id.Hex())
	if err != nil {
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	infoToDB := NewSongInfo(id, fh.Filename, int(fh.Size), metaData)
	err = songsColl.Insert(infoToDB)
	if err != nil {
		log.Println("Ошибка. При добавлении записи в БД: " + err.Error())
		removeFile(storageDirectory + id.Hex())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	log.Printf("Инфо. файл %v добавлен в систему\n", fh.Filename)

	w.Write([]byte("Файл успешно добавлен"))
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

	fmt.Printf("%v", n)
	if err != nil {
		log.Println("Ошибка. При поиске записи в БД: " + err.Error())
		return false, err
	}

	if n == 0 {
		return false, nil
	}

	return true, nil
}
