package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/STEJLS/AudioServer/flac"
	"github.com/STEJLS/AudioServer/mp3"
	"gopkg.in/mgo.v2/bson"
)

// addSong - добавляет новую песню в систему. Парсит метаданные о песне,
// заносит их в базу данных и сохраняет на диск (имя под которым хранится
//песня на диске – это id ее записи из базы данных).
func addSong(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на добавлене файла")

	w.Header().Add("Access-Control-Allow-Origin", "*")

	if r.Method == "OPTIONS" {
		return
	}

	w.Header().Add("Content-type", "text/html;charset=utf-8")
	fd, fh, err := r.FormFile(formFileName)
	if err != nil {
		log.Printf("Ошибка. Добавление песни не удалось. При поиске в форме файла с именем %q: %v\n", formFileName, err.Error())
		http.Error(w, "Файл не найден", http.StatusBadRequest)
		return
	}
	log.Printf("Инфо. Файл %v поступил на обработку\n", fh.Filename)

	extension := filepath.Ext(fh.Filename)
	var metaData IMetadata

	switch strings.ToLower(extension) {
	case ".mp3":
		metaData = mp3.ParseMetadata(fd)
		break
	case ".flac":
		metaData = flac.ParseMetadata(fd)
		break
	}

	if reflect.ValueOf(metaData).IsNil() {
		// if metaData == nil {
		log.Println("Инфо. Выход из запроса: данный формат не поддерживается: " + extension)
		http.Error(w, "Данный формат не поддерживается", http.StatusUnsupportedMediaType)
		return
	}
	log.Println("Инфо. Метаданные получены")

	id := bson.NewObjectId()
	infoToDB := NewSongInfo(id, fh.Filename, int(fh.Size), metaData)

	NormalizeMetadata(infoToDB, extension)

	flag, err := CheckExistMetaInDB(infoToDB)
	if err != nil {
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	if flag {
		log.Println("Инфо.Выход из запроса: данный файл уже есть в системе")
		http.Error(w, "Данный файл уже есть в системе", http.StatusBadRequest)
		return
	}

	err = saveFile(fd, storageDirectory+id.Hex())
	if err != nil {
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	err = songsColl.Insert(infoToDB)
	if err != nil {
		log.Println("Ошибка. При добавлении записи в БД: " + err.Error())
		removeFile(storageDirectory + id.Hex())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	w.Write([]byte("Файл успешно добавлен"))

	log.Println("Инфо. Закончилось выполнение запроса на добавлене файла")
}

// addPlayList - добавляет новый плэйлист в систему.
func addPlaylist(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на добавление плэйлиста")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	jsonIDs := r.FormValue("ids")
	name := r.FormValue("name")
	if jsonIDs == "" || name == "" {
		log.Printf("Инфо. На добавление в плэйлист поступили некоректные данные(пустые переменные)")
		http.Error(w, "Обнаружены незаполненные поля", http.StatusBadRequest)
		return
	}

	var ids []string
	err := json.Unmarshal([]byte(jsonIDs), &ids)
	if err != nil {
		log.Println("Ошибка. При анмаршалинге json: " + err.Error())
		http.Error(w, "Получены некорректные ID", http.StatusBadRequest)
	}

	playList := PlayList{
		ID:   bson.NewObjectId(),
		Name: name,
		IDs:  ids,
	}

	err = playListsColl.Insert(playList)
	if err != nil {
		log.Println("Ошибка. При добавлении записи в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	serveContent(playList.ID, w, r)
}

// getMetadataOfPopularSongs - отдает информацию о популярных песнях.
func getMetadataOfPopularSongs(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу популярных песен")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	count := getCountOfMetadata(r)

	var result []SongInfo
	err := songsColl.Find(nil).Sort("-CountOfDownload").Limit(count).All(&result)
	if err != nil {
		log.Println("Ошибка. При поиске популярных песен в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	serveContent(result, w, r)
}

// getMetadataOfNewSongs - отдает информацию о новых песнях.
func getMetadataOfNewSongs(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу новинок")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	count := getCountOfMetadata(r)

	var result []SongInfo
	err := songsColl.Find(nil).Sort("-UploadDate").Limit(count).All(&result)
	if err != nil {
		log.Println("Ошибка. При поиске новинок в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	serveContent(result, w, r)
}

// getMetadataOfSongsbyIDs - отдает информацию об указанных в теле запроса песнях.
func getMetadataOfSongsbyIDs(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу метаданные об массиве песен")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	jsonIDs := r.FormValue("ids")
	if jsonIDs == "" {
		log.Printf(fmt.Sprintf("Инфо. На выход посутпила пустая строка ids"))
		http.Error(w, "Полученненно не IDs, а пустая строка", http.StatusBadRequest)
		return
	}

	ids := jsonIDsToSliceObjectIDs(jsonIDs)
	if ids == nil || len(ids) == 0 {
		http.Error(w, "Получены некорректные ID", http.StatusBadRequest)
		return
	}

	var result []SongInfo

	err := songsColl.Find(bson.M{"_id": bson.M{"$in": ids}}).All(&result)
	if err != nil {
		log.Println("Ошибка. При поиске песен в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	serveContent(result, w, r)
}

// getPlaylists - отдает информацию о новых плейлистах.
func getPlaylists(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу плэйлистов")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	count := getCountOfMetadata(r)

	var result []PlayList
	err := playListsColl.Find(nil).Sort("-_id").Limit(count).All(&result)
	if err != nil {
		log.Println("Ошибка. При поиске плэйлистов в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	serveContent(result, w, r)
}

// searchSongs - осуществляет поиск песен в базе данных по полученной
// из тела запроса строке и возвращает информацию о найденных песнях.
func searchSongs(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на поиск песен")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	stringForSearch := r.FormValue("searchString")
	if stringForSearch == "" {
		log.Printf("Инфо. На поиск поcтупила пустая строка")
		data, _ := json.Marshal(nil)
		w.Write(data)
		return
	}

	stringForSearchInDB := strings.Join(strings.Fields(regexp.QuoteMeta(stringForSearch)), "|")
	log.Printf("Инфо. Поиск по строке: " + stringForSearchInDB)

	var result []SongInfo

	err := songsColl.Find(bson.M{"$or": []bson.M{bson.M{"Artist": bson.RegEx{Pattern: stringForSearchInDB, Options: "i"}},
		bson.M{"Genre": bson.RegEx{Pattern: stringForSearchInDB, Options: "i"}},
		bson.M{"Title": bson.RegEx{Pattern: stringForSearchInDB, Options: "i"}}}}).All(&result)
	if err != nil {
		log.Println("Ошибка. При поиске в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	words := strings.Fields(stringForSearch)

	if len(words) != 1 {
		result = *advancedSearch(words, &result)
	}

	serveContent(result, w, r)
}

// searchPlaylists - Осуществляет поиск плейлистов в базе данных по
// полученной из тела запроса строке и возвращает информацию о найденных плелистах.
func searchPlaylists(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на поиск плейлистов")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	stringForSearch := r.FormValue("searchString")
	if stringForSearch == "" {
		log.Printf("Инфо. На поиск поcтупила пустая строка")
		data, _ := json.Marshal(nil)
		w.Write(data)
		return
	}

	stringForSearch = strings.Join(strings.Fields(regexp.QuoteMeta(stringForSearch)), ".*")
	log.Printf("Инфо. Поиск по строке: " + stringForSearch)

	var result []PlayList

	err := playListsColl.Find(bson.M{"Name": bson.RegEx{Pattern: stringForSearch, Options: "i"}}).All(&result)
	if err != nil {
		log.Println("Ошибка. При поиске в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	serveContent(result, w, r)
}

// getSong - Отдает на скачивание песню по запрошенному id.
// Используется так же для прослушивания песни на сайте.
// Прослушивание не считается за скачивание.
func getSong(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу файла")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	download := r.FormValue("isDownload")

	id := r.FormValue("id")
	if id == "" {
		log.Println("Ошибка. id не найден")
		http.Error(w, "id не найден", http.StatusBadRequest)
		return
	}

	if !bson.IsObjectIdHex(id) {
		log.Printf("Ошибка. Полученное значение не является ID(id = %q) ", id)
		http.Error(w, "Получен некорректный ID", http.StatusBadRequest)
		return
	}

	var result SongInfo

	err := songsColl.FindId(bson.ObjectIdHex(id)).One(&result)
	if err != nil {
		if err.Error() == "not found" {
			log.Println("Инфо. Запрашиваемой песни нет в БД: " + err.Error())
			http.Error(w, "Такой песни нет", http.StatusBadRequest)
			return
		}

		log.Println("Ошибка. При поиске записи в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Disposition", "filename=\""+result.FileName+"\"")
	http.ServeFile(w, r, storageDirectory+id)

	log.Println("Инфо. Закончилось выполнение запроса на отдачу файла")

	if download == "true" {
		err = songsColl.UpdateId(bson.ObjectIdHex(id), bson.M{"$set": bson.M{"CountOfDownload": result.CountOfDownload + 1}})
		if err != nil {
			log.Println("Ошибка. При обновлении записи(" + id + ") - увеличивалось кол-во скачаваний: " + err.Error())
		}
	}
}

// getSongsInZip - отдает на скачивание указанные в теле запроса песни, упакованные в zip архив.
func getSongsInZip(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу песен в zip")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	jsonIDs := r.FormValue("ids")
	if jsonIDs == "" {
		log.Printf(fmt.Sprintf("Инфо. На выход посутпил пустой ids"))
		http.Error(w, "Получено не IDs, а пустая строка", http.StatusBadRequest)
		return
	}

	ids := jsonIDsToSliceObjectIDs(jsonIDs)
	if ids == nil || len(ids) == 0 {
		http.Error(w, "Получены некорректные ID", http.StatusBadRequest)
		return
	}

	serveSongsInZIP(ids, serviceName+time.Now().Format("15:04:05.000")+".zip", w)

	log.Println("Инфо. Закончилось успешно выполнение запроса на отдачу плэйлистов в zip")
}

// getPlaylistInZip - отдает на скачивание песни указанного плейлиста,
// упакованные в zip архив.
func getPlaylistInZip(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу плэйлистов в zip")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	id := r.FormValue("id")

	if id == "" {
		log.Printf("Инфо. Получен не ID, а пустая строка")
		http.Error(w, "Получен не ID, а пустая строка", http.StatusBadRequest)
		return
	}

	if !bson.IsObjectIdHex(id) {
		log.Printf(fmt.Sprintf("Инфо. На выход посутпил некорректный id(%v)", id))
		http.Error(w, "Получен некорректный ID", http.StatusBadRequest)
		return
	}

	var playList PlayList
	err := playListsColl.FindId(bson.ObjectIdHex(id)).One(&playList)
	if err != nil {
		if err.Error() == "not found" {
			log.Println("Инфо. Запрашиваемого плэйлиста нет в БД: " + err.Error())
			http.Error(w, "С полученным ID в БД записи не существует", http.StatusBadRequest)
			return
		}

		log.Println("Ошибка. При поиске записи в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	serveSongsInZIP(makeSliceSliceObjectIDs(playList.IDs), playList.Name+".zip", w)
}
