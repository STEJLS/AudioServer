package main

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/STEJLS/AudioServer/mp3"
	"gopkg.in/mgo.v2/bson"
)

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

	w.Header().Add("Content-Disposition", "filename=\""+result.FileName+"\"")
	http.ServeFile(w, r, storageDirectory+id)

	log.Println("Инфо. Закончилось выполнение запроса на отдачу файла")

	err = songsColl.UpdateId(bson.ObjectIdHex(id), bson.M{"$set": bson.M{"CountOfDownload": result.CountOfDownload + 1}})
	if err != nil {
		log.Println("Ошибка. При обновлении записи(" + id + ") - увеличивалось кол-во скачаваний: " + err.Error())
	}
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

	err := songsColl.Find(bson.M{"$or": []bson.M{bson.M{"Artist": bson.RegEx{Pattern: stringForSearch, Options: "i"}},
		bson.M{"Genre": bson.RegEx{Pattern: stringForSearch, Options: "i"}},
		bson.M{"Title": bson.RegEx{Pattern: stringForSearch, Options: "i"}}}}).All(&result)
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