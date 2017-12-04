package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/STEJLS/AudioServer/flac"
	"github.com/STEJLS/AudioServer/mp3"
	"gopkg.in/mgo.v2/bson"
)

// addSong - добавляет метаданные о песни в базу данных, копирует файл на диск в директорию,
// путь до которой хранится в переменной storageDirectory (имя песни - id записи из БД)
// Файл извлекается из тела POST запроса с именем, хранящимся в переменной formFileName.
// Если файл добавлен ответ - "Файл успешно добавлен" - 200
// Если не получилось извлечь файл из формы ответ - "Файл не найден" - 400
// Если добавляемый файл уже есть в системе - "Загружаемый файл уже есть в системе" - 400
// Если тип файла не поддерижвается ответ - "Данный формат не поддерживается" - 415
// Если произошла внутренняя ошибка сервера - "Неполадки на сервере, повторите попытку позже" - 500
func addSong(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на добавлене файла")

	w.Header().Add("Access-Control-Allow-Origin", "*")
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

	switch extension {
	case ".mp3":
		metaData = mp3.ParseMetadata(fd)
		break
	case ".flac":
		metaData = flac.ParseMetadata(fd)
		break
	}

	if metaData == nil {
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

// addPlayList - добавляет плэйлист в систему
// Назавние плейлиста извлекается из переменной запросы 'name' - это строка .
// Список id песен плейлиста извлекается из переменной запросы 'ids' - это массив строк в формате json.
// Если если вместо ids или name пустая строка  - "Обнаружены незаполненные поля" - 400.
// Если произошла ошибка при анмаршалинге ids  - "Получены некорректные ID" - 400.
// Если произошла внутренняя ошибка сервера - "Неполадки на сервере, повторите попытку позже" - 500.
func addPlaylist(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на добавление плэйлиста")

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

// getMetadataOfPopularSongs - считывает переменную count из запроса
// и возвращает count методанных о популярных песнях в формате json.
// Если count не указан или некорректный, то используется значение по умолчанию.
// Если произошла внутренняя ошибка сервера - "Неполадки на сервере, повторите попытку позже" - 500.
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

	serveContent(result, w, r)
}

// getMetadataOfNewSongs - считывает переменную count из запроса
// и возвращает count методанных о новых песнях в формате json.
// Если count не указан или некорректный, то используется значение по умолчанию.
// Если произошла внутренняя ошибка сервера - "Неполадки на сервере, повторите попытку позже" - 500.
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

	serveContent(result, w, r)
}

// getMetadataOfSongsbyIDs - считывает переменную ids из запроса, в которой должен быть массив id в формате json,
// и возвращает массив метаданных об этих песнях.
// Если если вместо ids пустая строка  - "Полученненно не IDs, а пустая строка" - 400.
// Если получен некорретные id  - "Получены некорректные ID" - 400.
// Если произошла внутренняя ошибка сервера - "Неполадки на сервере, повторите попытку позже" - 500.
func getMetadataOfSongsbyIDs(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу метаданные об массиве песен")

	jsonIDs := r.FormValue("ids")
	if jsonIDs == "" {
		log.Printf(fmt.Sprintf("Инфо. На выход посутпила пустая строка ids"))
		http.Error(w, "Полученненно не IDs, а пустая строка", http.StatusBadRequest)
		return
	}

	ids := jsonIDsToSliceObjectIDs(jsonIDs)
	if ids == nil || len(ids) == 0 {
		http.Error(w, "Получены некорректные ID", http.StatusBadRequest)
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

// getPlaylists - считывает переменную count из запроса и возвращает count методанных о плэйлистах в формате json.
// Если count не указан или некорректный, то используется значение по умолчанию.
// Если произошла внутренняя ошибка сервера - "Неполадки на сервере, повторите попытку позже" - 500.
func getPlaylists(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу плэйлистов")

	count := getCountOfMetadata(r)

	var result []PlayList
	err := playListsColl.Find(nil).Limit(count).All(&result)
	if err != nil {
		log.Println("Ошибка. При поиске плэйлистов в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	serveContent(result, w, r)
}

// searchSongs - считывает переменную searchString из запроса,
// осуществляет поиск песен в базе данных по этой строке и возвращает метаданные о песнях в формате json
// Если если вместо searchString пустая строка  - "null" - 200.
// Если по введенной строке ничего не найдено - "null" - 200.
// Если произошла внутренняя ошибка сервера - "Неполадки на сервере, повторите попытку позже" - 500.
func searchSongs(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на поиск песен")

	stringForSearch := r.FormValue("searchString")
	if stringForSearch == "" {
		log.Printf("Инфо. На поиск поcтупила пустая строка")
		data, _ := json.Marshal(nil)
		w.Write(data)
		return
	}

	stringForSearch = strings.Join(strings.Fields(regexp.QuoteMeta(stringForSearch)), "|")
	log.Printf("Инфо. Поиск по строке: " + stringForSearch)

	var result []SongInfo

	err := songsColl.Find(bson.M{"$or": []bson.M{bson.M{"Artist": bson.RegEx{Pattern: stringForSearch, Options: "i"}},
		bson.M{"Genre": bson.RegEx{Pattern: stringForSearch, Options: "i"}},
		bson.M{"Title": bson.RegEx{Pattern: stringForSearch, Options: "i"}}}}).All(&result)
	if err != nil {
		log.Println("Ошибка. При поиске в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	serveContent(result, w, r)
}

// searchPlaylists - считывает переменную searchString из запроса,
// осуществляет поиск плэйлистов в базе данных по этой строке и возвращает метаданные о плейлисте в формате json
// Если если вместо searchString пустая строка  - "null" - 200.
// Если по введенной строке ничего не найдено - "null" - 200.
// Если произошла внутренняя ошибка сервера - "Неполадки на сервере, повторите попытку позже" - 500.
func searchPlaylists(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на поиск плейлистов")

	stringForSearch := r.FormValue("searchString")
	if stringForSearch == "" {
		log.Printf("Инфо. На поиск поcтупила пустая строка")
		data, _ := json.Marshal(nil)
		w.Write(data)
		return
	}

	stringForSearch = strings.Join(strings.Fields(regexp.QuoteMeta(stringForSearch)), " ")
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

// getSong - отдает на скачивание песню по запрошенному ID.
// ID извлекается переменной 'id' из тела запроса.
// Если если вместо id пустая строка  - "ID не найден" - 400.
// Если получен некорретные id  - "Полученное значение не является ID" - 400.
// Если получен некорретные id, но песни с таким id нет  - "С полученным ID в БД не существует записи" - 400.
// Если произошла внутренняя ошибка сервера - "Неполадки на сервере, повторите попытку позже" - 500.
func getSong(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу файла")

	download := r.FormValue("isDownload")

	id := r.FormValue("id")
	if id == "" {
		log.Println("Ошибка. ID не найден")
		http.Error(w, "ID не найден", http.StatusBadRequest)
		return
	}

	if !bson.IsObjectIdHex(id) {
		log.Printf("Ошибка. Полученное значение не является ID(id = %q) ", id)
		http.Error(w, "Полученное значение не является ID", http.StatusBadRequest)
		return
	}

	var result SongInfo

	err := songsColl.FindId(bson.ObjectIdHex(id)).One(&result)
	if err != nil {
		if err.Error() == "not found" {
			log.Println("Инфо. Запрашиваемой песни нет в БД: " + err.Error())
			http.Error(w, "С полученным ID в БД не существует записи", http.StatusBadRequest)
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

// getSongsInZip - на вход принимает массив ID песен в формате json,
// который находится в переменной запроса с именем 'ids'.
// Отдает на скачивание указанные песни в архиве zip.
// Если если вместо ids пустая строка  - "Полученненно не IDs, а пустая строка" - 400.
// Если получен некорретные id  - "Получены некорректные ID" - 400.
// Если произошла внутренняя ошибка сервера - "Неполадки на сервере, повторите попытку позже" - 500.
func getSongsInZip(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу песен в zip")

	jsonIDs := r.FormValue("ids")
	if jsonIDs == "" {
		log.Printf(fmt.Sprintf("Инфо. На выход посутпил пустой ids"))
		http.Error(w, "Полученненно не IDs, а пустая строка", http.StatusBadRequest)
		return
	}

	ids := jsonIDsToSliceObjectIDs(jsonIDs)
	if ids == nil || len(ids) == 0 {
		http.Error(w, "Получены некорректные ID", http.StatusBadRequest)
	}

	serveSongsInZIP(ids, serviceName+time.Now().Format("15:04:05.000")+".zip", w)

	log.Println("Инфо. Закончилось успешно выполнение запроса на отдачу плэйлистов в zip")
}

// getPlaylistInZip - на вход принимает ID плейлиста, который находится в переменной запроса с именем 'id'.
// Отдает на скачивание песни указанного плейлиста в архиве zip.
// Если получен некорретный id  - "Полученненн неверный ID" - 400.
// Если указанного id нет в БД - "С полученным ID в БД не существует записи" - 400.
// Если произошла внутренняя ошибка сервера - "Неполадки на сервере, повторите попытку позже" - 500.
func getPlaylistInZip(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу плэйлистов в zip")

	id := r.FormValue("id")
	if id == "" || !bson.IsObjectIdHex(id) {
		log.Printf(fmt.Sprintf("Инфо. На выход посутпил некорректный id(%v)", id))
		http.Error(w, "Полученненн неверный ID", http.StatusBadRequest)
		return
	}

	var playList PlayList
	err := playListsColl.FindId(bson.ObjectIdHex(id)).One(&playList)
	if err != nil {
		if err.Error() == "not found" {
			log.Println("Инфо. Запрашиваемого плэйлиста нет в БД: " + err.Error())
			http.Error(w, "С полученным ID в БД не существует записи", http.StatusBadRequest)
			return
		}

		log.Println("Ошибка. При поиске записи в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	serveSongsInZIP(makeSliceSliceObjectIDs(playList.IDs), playList.Name+".zip", w)
}
