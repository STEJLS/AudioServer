package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

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
	playListsColl = audioDBsession.DB(DBName).C("Playlists")

	log.Printf("Инфо. Подключение к базе данных установлено.")
}

// saveFile - сохраняет копию обрабатываемой песни на диске под именем newFileName,
// если все прошло успешно, то возвращает nil, в противном случае объект ошибки
func saveFile(readSeeker io.ReadSeeker, newFileName string) error {
	_, err := readSeeker.Seek(0, os.SEEK_SET)
	if err != nil {
		log.Println("Ошибка. Выход из запроса: не удалось перейти на начало копируемого файла: " + err.Error())
		return err
	}
	data, err := ioutil.ReadAll(readSeeker)
	if err != nil {
		log.Println("Ошибка. Выход из запроса: не удалось прочитать файл пользователя: " + err.Error())
		return err
	}

	err = ioutil.WriteFile(newFileName, data, 0666)
	if err != nil {
		log.Println("Ошибка. Выход из запроса: не удалось создать новый файл: " + err.Error())
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
func CheckExistMetaInDB(mataData *SongInfo) (bool, error) {
	n, err := songsColl.Find(bson.M{"Title": mataData.Title,
		"Artist":   mataData.Artist,
		"Genre":    mataData.Genre,
		"Bitrate":  mataData.Bitrate,
		"Duration": mataData.Duration,
		"Size":     mataData.Size,
	}).Count()

	if err != nil {
		log.Println("Ошибка.Выход из запроса: при поиске записи в БД: " + err.Error())
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
	strCount := r.FormValue("count")

	if strCount == "" {
		log.Printf("Инфо. Количетсво запрашиваемых записей не указано, отдаю стандартное кол-во: %v", defaultCountMatadataForUpload)
		return defaultCountMatadataForUpload
	}

	count, err := strconv.Atoi(strCount)
	if err != nil {
		log.Println("Ошибка. Не получилось преобрахзовать введенное кол-во в число: " + err.Error())
		return defaultCountMatadataForUpload
	}

	if count < 0 {
		return defaultCountMatadataForUpload
	}

	return count
}

// NormalizeMetadata - проверяет пустые ли поля исполнитель и название
// и в таком случае пытает их вычислить.
// Проверяет пустое ли поле жанр, если да - ставит заглушку
func NormalizeMetadata(infoToDB *SongInfo, ext string) {
	if infoToDB.Artist == "" && infoToDB.Title == "" {
		tryParseTitleAndArtistFromFileName(infoToDB, ext)
	}

	if infoToDB.Genre == "" {
		infoToDB.Genre = "Other"
	}
}

// tryParseTitleAndArtistFromFileName - пытается из имени файла получить исполнителя
// и название песни. Разделение идет по символу"-"
func tryParseTitleAndArtistFromFileName(song *SongInfo, ext string) {
	fileNameWithoutExt := strings.TrimSuffix(song.FileName, ext)
	n := strings.Index(fileNameWithoutExt, "-")
	if n != -1 {
		song.Artist = strings.TrimSpace(fileNameWithoutExt[0:n])
		song.Title = strings.TrimSpace(fileNameWithoutExt[n+1 : len(fileNameWithoutExt)])
	} else {
		song.Title = fileNameWithoutExt
	}
}

// serveSongsInZIP - отдает на скачивание песни, упакованные в zip архив.
func serveSongsInZIP(ids []bson.ObjectId, fileName string, w http.ResponseWriter) {
	var result []SongInfo

	err := songsColl.Find(bson.M{"_id": bson.M{"$in": ids}}).All(&result)
	if err != nil {
		log.Println("Ошибка. При поиске песен в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	if len(result) == 0 {
		log.Println("Ошибка. Ни одна песня из полученного массива id не найдена в бд: " + err.Error())
		http.Error(w, "Указанные песни не найдены", http.StatusBadRequest)
		return
	}

	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	for _, song := range result {
		data, err := ioutil.ReadFile(storageDirectory + song.ID.Hex())
		if err != nil {
			log.Printf("Ошибка. При чтении файла c диска id = %v ошибка: %v", song.ID.Hex(), err.Error())
			continue
		}

		fileWriter, err := zipWriter.Create(song.FileName)
		if err != nil {
			log.Println("Ошибка. При создании нового файла в архиве ошибка: " + err.Error())
			continue
		}

		_, err = fileWriter.Write(data)
		if err != nil {
			log.Println("Ошибка. При записи в файл архива: " + err.Error())
			continue
		}
	}
	err = zipWriter.Close()
	if err != nil {
		log.Println("Ошибка. При закрытии архива: " + err.Error())
	}

	w.Header().Add("Content-Disposition", "filename=\""+fileName+"\"")
	w.Header().Add("Content-type", "application/zip")
	w.Header().Add("Content-Length", fmt.Sprintf("%v", buf.Len()))

	_, err = w.Write(buf.Bytes())
	if err != nil {
		log.Println("Ошибка. При отдачи метоинформации: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}
	log.Println("Инфо. Песни в формате zip успешно отправлены")

	_, err = songsColl.UpdateAll(bson.M{"_id": bson.M{"$in": ids}}, bson.M{"$inc": bson.M{"CountOfDownload": 1}})
	if err != nil {
		log.Println("Ошибка. При инкременте поля CountOfDownload: " + err.Error())
	} else {
		log.Println("Инфо. Успешно увеличино кол-во загрузок для скачиваемых песен")
	}
}

// jsonIDsToSliceObjectIDs - принимает на вход строку, в которой записан массив строк в формате json,
// делает анмаршаллинг json'а и проверяет каждую строку,
// является ли она bson.ObjectId, и в случае успешнйо проверки добавляет ее к возвращаемому массиву
func jsonIDsToSliceObjectIDs(jsonIDs string) []bson.ObjectId {
	var ids []string
	err := json.Unmarshal([]byte(jsonIDs), &ids)
	if err != nil {
		log.Println("Ошибка. При анмаршалинге json: " + err.Error())
		return nil
	}

	return makeSliceSliceObjectIDs(ids)
}

// makeSliceSliceObjectIDs - принимает на вход массив строк, проверяет каждую строку,
// является ли она bson.ObjectId, и в случае успешнйо проверки добавляет ее к возвращаемому массиву.
func makeSliceSliceObjectIDs(ids []string) []bson.ObjectId {
	arr := make([]bson.ObjectId, 0, len(ids))
	for _, id := range ids {
		if bson.IsObjectIdHex(id) {
			arr = append(arr, bson.ObjectIdHex(id))
		}
	}

	return arr
}

//serveContent - принимает на вход данные, переводит их в формат json и пишет их в ResponseWriter
func serveContent(inData interface{}, w http.ResponseWriter, r *http.Request) {
	data, err := json.Marshal(inData)
	if err != nil {
		log.Println("Ошибка. При маршалинге в json результата: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-type", "application/json;")

	_, err = w.Write(data)
	if err != nil {
		log.Println("Ошибка. При отдачи метоинформации: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	log.Println("Инфо. Отдача метаданных успешно закончена")
}

// advancedSearch - примает на вход массив слов, по которому будет осуществляться
// углубленный поиск и массив метаинформации, уже найденной в бд.
// Возвращает массив метаинформации, в котором в каждой записи есть каждое слово из входного массива слов.
func advancedSearch(words []string, result *[]SongInfo) *[]SongInfo {
	checkedResult := make([]SongInfo, 0, len(*result))

	for _, item := range *result {
		flag := true
		sample := strings.ToLower(item.Artist + item.Title + item.Genre)

		for _, item := range words {
			lowerItem := strings.ToLower(item)
			if !strings.Contains(sample, lowerItem) {
				flag = false
				break
			}
		}

		if flag {
			checkedResult = append(checkedResult, item)
		}

	}

	return &checkedResult
}
