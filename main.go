package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
	formFileName                  string = "file"      // имя файла в форме на сайте
	storageDirectory              string = "../music/" // место для хранения песен
	initialCountOfDownloads       int64  = 0           // начальное  количесвто скачиваний
	defaultCountMatadataForUpload int    = 25          // кол-во по умолчанию сколько метаданных будет отдаваться
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
	}

	http.HandleFunc("/addSong", addSong)
	http.HandleFunc("/getSong", getSong)
	http.HandleFunc("/getMetadataOfNewSongs", getMetadataOfNewSongs)
	http.HandleFunc("/getMetadataOfPopularSongs", getMetadataOfPopularSongs)
	http.HandleFunc("/searchSongs", searchSongs)
	http.HandleFunc("/addSongForm", addSongForm)
	http.HandleFunc("/getSongForm", getSongForm)
	http.HandleFunc("/getPopularSongsForm", getPopularSongsForm)

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

func getSongForm(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`<html>
		<head>
		<meta http-equiv="Content-Type" content="text/html; charset=utf-8" />
		<title>AudioServer</title>
		</head>
		<body>
		<form action="/getSong" 
		method="post" enctype="application/x-www-form-urlencoded">
		<input type="text" name="id">
		<input type="submit"/>
		</form>
		</body>
		</html>`),
	)
}

func getPopularSongsForm(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`<html>
		<head>
		<meta http-equiv="Content-Type" content="text/html; charset=utf-8" />
		<title>AudioServer</title>
		</head>
		<body>
		<form action="/getMetadataOfPopularSongs" 
		method="post" enctype="application/x-www-form-urlencoded">
		<input type="text" name="count">
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

	w.Header().Add("Access-Control-Allow-Origin", "*")
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

	if err != nil {
		log.Println("Ошибка. При поиске записи в БД: " + err.Error())
		return false, err
	}

	if n == 0 {
		return false, nil
	}

	return true, nil
}

// getSong - отдает песню по запрошенному ID
// Возможные http статусы: 200, 400, 500
func getSong(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу файла")

	id := r.FormValue("id")
	if id == "" {
		log.Println("Ошибка. ID не найден")
		http.Error(w, "ID не найден", http.StatusBadRequest)
		return
	}

	if !bson.IsObjectIdHex(id) {
		log.Println("Ошибка. Полученное значение не является ID: " + id)
		http.Error(w, "Полученное значение не является ID", http.StatusBadRequest)
		return
	}

	var result SongInfo

	err := songsColl.FindId(bson.ObjectIdHex(id)).One(&result)
	if err != nil {
		if err.Error() == "not found" {
			log.Println("Инфо. Запращиваемой песни нет в БД: " + err.Error())
			http.Error(w, "С полученным ID в БД не существует записи", http.StatusBadRequest)
			return
		}

		log.Println("Ошибка. При поиске записи в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	data, err := ioutil.ReadFile(storageDirectory + id)
	if err != nil {
		log.Println("Ошибка. При чтении файла(" + id + ") с диска :" + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	w.Header().Add("Content-Disposition", "filename=\""+result.FileName+"\"")
	w.Header().Add("Content-Type", mime.TypeByExtension(filepath.Ext(result.FileName)))
	w.Header().Add("Accept-Ranges", "bytes")
	w.Header().Add("Content-Length", fmt.Sprintf("%v", len(data)))

	_, err = w.Write(data)
	if err != nil {
		log.Println("Ошибка. При отдачи файла(" + id + "): " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	log.Println("Инфо. Закончилось выполнение запроса на отдачу файла")

	err = songsColl.UpdateId(bson.ObjectIdHex(id), bson.M{"$set": bson.M{"CountOfDownload": result.CountOfDownload + 1}})
	if err != nil {
		log.Println("Ошибка. При обновлении записи(" + id + ") - увеличивалось кол-во скачаваний: " + err.Error())
	}
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

//getMetadataOfPopularSongs - отдает методанные о популярных песнях в формате json
func getMetadataOfPopularSongs(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу популярных песен")

	count := getCountOfMetadata(r)

	var result []SongInfo
	err := songsColl.Find(nil).Sort("-CountOfDownload").Limit(count).All(&result)
	if err != nil {
		log.Println("Ошибка. При поиске популярных песен в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	if len(result) == 0 {
		return
	}

	data, err := json.Marshal(result)
	if err != nil {
		log.Println("Ошибка. При маршалинге в json популярных песен: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-type", "application/json;")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	_, err = w.Write(data)
	if err != nil {
		log.Println("Ошибка. При отдачи метоинформации: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	log.Println("Инфо. Закончилось успешно выполнение запроса на отдачу популярных песен")
}

// getMetadataOfNewSongs - отдает методанные о 15 последних добвленных песен в формате json
func getMetadataOfNewSongs(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу новинок")

	count := getCountOfMetadata(r)

	var result []SongInfo
	err := songsColl.Find(nil).Sort("-UploadDate").Limit(count).All(&result)
	if err != nil {
		log.Println("Ошибка. При поиске новинок в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	if len(result) == 0 {
		return
	}

	data, err := json.Marshal(result)
	if err != nil {
		log.Println("Ошибка. При маршалинге в json новинок: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Content-type", "application/json;")

	_, err = w.Write(data)
	if err != nil {
		log.Println("Ошибка. При отдачи метоинформации: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	log.Println("Инфо. Закончилось успешно выполнение запроса на отдачу новинок")
}

// searchSongs - осуществляет поиск песен в базе данных и возвращает метаданные в json
// При вводе пустой строки ответ 400  статус.
// Если по введенной строке ничего не найдено, то возвращается пустота и 200 статус.
func searchSongs(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на поиск песен")

	stringForSearch := r.FormValue("searchString")
	if stringForSearch == "" {
		log.Printf("Инфо. На поиск поcтупила некорректная строка")
		http.Error(w, "Полученная строка не может использоваться для поиска", http.StatusBadRequest)
		return
	}

	stringForSearch = strings.Join(strings.Fields(regexp.QuoteMeta(stringForSearch)), " ")
	log.Printf("Инфо. Поиск по строке: " + stringForSearch)

	var result []SongInfo

	err := songsColl.Find(bson.M{"$or": []bson.M{bson.M{"Artist": bson.RegEx{stringForSearch, "i"}},
		bson.M{"Genre": bson.RegEx{stringForSearch, "i"}},
		bson.M{"Title": bson.RegEx{stringForSearch, "i"}}}}).All(&result)
	if err != nil {
		log.Println("Ошибка. При поиске в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	if len(result) == 0 {
		return
	}

	data, err := json.Marshal(result)
	if err != nil {
		log.Println("Ошибка. При маршалинге в json результата поиска: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Content-type", "application/json;")

	_, err = w.Write(data)
	if err != nil {
		log.Println("Ошибка. При отдачи метоинформации: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	log.Println("Инфо. Закончилось успешно выполнение запроса поиск песен")
}
