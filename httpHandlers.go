package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
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

// addSong - добавляет песню в базу данных, копирует файл на диск в папку /music(имя песни - id записи из БД)
// Если файл добавлен ответ - "Файл успешно добавлен" - 200
// Если не получилось извлечь файл из формы ответ - "Файл не найден" - 400
// Если добавляемый файл уже есть в системе - "Загружаемый файл уже есть в системе" - 400
// Если тип файла не поддерижвается ответ - "Данный формат не поддерживается" - 415
// Если при создании копии на диске произошла ошибка ответ - "Неполадки на сервере, повторите попытку позже" - 500
// Если при записи метаданных в БД произошла ошибка ответ - "Неполадки на сервере, повторите попытку позже" - 500
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

	if infoToDB.Artist == "" && infoToDB.Title == "" {
		tryParseTitleAndArtistFromFileName(infoToDB, extension)
	}

	if infoToDB.Genre == "" {
		infoToDB.Genre = "Other"
	}

	err = songsColl.Insert(infoToDB)
	if err != nil {
		log.Println("Ошибка. При добавлении записи в БД: " + err.Error())
		removeFile(storageDirectory + id.Hex())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	log.Printf("Инфо. файл %v добавлен в систему\n", fh.Filename)

	_, err = w.Write([]byte("Файл успешно добавлен"))
	if err != nil {
		log.Println("Ошибка. При отдачи метоинформации: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	log.Println("Инфо. Закончилось выполнение запроса на отдачу файла")
}

// getSong - отдает песню по запрошенному ID
// Возможные http статусы: 200, 400
func getSong(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу файла")

	download := r.FormValue("download")

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

//getMetadataOfPopularSongs - отдает методанные о популярных песнях в формате json
func getMetadataOfPopularSongs(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу популярных песен")

	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Content-type", "application/json;")

	count := getCountOfMetadata(r)

	var result []SongInfo
	err := songsColl.Find(nil).Sort("-CountOfDownload").Limit(count).All(&result)
	if err != nil {
		log.Println("Ошибка. При поиске популярных песен в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	data, err := json.Marshal(result)
	if err != nil {
		log.Println("Ошибка. При маршалинге в json популярных песен: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	_, err = w.Write(data)
	if err != nil {
		log.Println("Ошибка. При отдачи метоинформации: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	log.Println("Инфо. Закончилось успешно выполнение запроса на отдачу популярных песен")
}

// getMetadataOfNewSongs - отдает методанные о последних добвленных песен в формате json
func getMetadataOfNewSongs(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу новинок")

	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Content-type", "application/json;")

	count := getCountOfMetadata(r)

	var result []SongInfo
	err := songsColl.Find(nil).Sort("-UploadDate").Limit(count).All(&result)
	if err != nil {
		log.Println("Ошибка. При поиске новинок в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	data, err := json.Marshal(result)
	if err != nil {
		log.Println("Ошибка. При маршалинге в json новинок: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

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

	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Content-type", "application/json;")

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

	data, err := json.Marshal(result)
	if err != nil {
		log.Println("Ошибка. При маршалинге в json результата поиска: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	_, err = w.Write(data)
	if err != nil {
		log.Println("Ошибка. При отдачи метоинформации: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	log.Println("Инфо. Закончилось успешно выполнение запроса поиск песен")
}

// addPlayList - добавляет плэйлист в систему
func addPlaylist(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на добавление плэйлиста")

	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Content-type", "text/html;charset=utf-8")

	jsonIDs := r.FormValue("ids")
	name := r.FormValue("name")
	if jsonIDs == " " || name == "" {
		log.Printf("Инфо. На добавление в плэйлист поступили некоректные данные")
		http.Error(w, "Введены неверные данные", http.StatusBadRequest)
		return
	}

	var ids []string
	err := json.Unmarshal([]byte(jsonIDs), &ids)
	if err != nil {
		log.Println("Ошибка. При анмаршалинге json: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	id := bson.NewObjectId()

	playList := PlayList{
		ID:   id,
		Name: name,
		IDs:  ids,
	}

	err = playListsColl.Insert(playList)
	if err != nil {
		log.Println("Ошибка. При добавлении записи в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	data, err := json.Marshal(id)
	if err != nil {
		log.Println("Ошибка. При маршалинге в json: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	_, err = w.Write(data)
	if err != nil {
		log.Println("Ошибка. При отдачи метоинформации: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	log.Println("Инфо. Закончилось успешно выполнение запроса на добавление плэйлиста")
}

// getPlaylists - отдает методанные о плэйлистах в формате json
func getPlaylists(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу плэйлистов")

	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Content-type", "application/json;")

	count := getCountOfMetadata(r)

	var result []PlayList
	err := playListsColl.Find(nil).Limit(count).All(&result)
	if err != nil {
		log.Println("Ошибка. При поиске плэйлистов в БД: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	data, err := json.Marshal(result)
	if err != nil {
		log.Println("Ошибка. При маршалинге в json плэйлистов: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	_, err = w.Write(data)
	if err != nil {
		log.Println("Ошибка. При отдачи метоинформации: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	log.Println("Инфо. Закончилось успешно выполнение запроса на отдачу плэйлистов")
}

// searchPlaylists - осуществляет поиск плэйлистов в базе данных и возвращает метаданные в json
// При вводе пустой строки ответ 400  статус.
// Если по введенной строке ничего не найдено, то возвращается пустота и 200 статус.
func searchPlaylists(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на поиск плейлистов")

	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Content-type", "application/json;")

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

	data, err := json.Marshal(result)
	if err != nil {
		log.Println("Ошибка. При маршалинге в json результата поиска: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	_, err = w.Write(data)
	if err != nil {
		log.Println("Ошибка. При отдачи метоинформации: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	log.Println("Инфо. Закончилось успешно выполнение запроса на поиск плейлистов")
}

// getPlaylistInZip - создает zip архив, который содержит в себе песни указанного плэйлиста
func getPlaylistInZip(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу плэйлистов в zip")

	id := r.FormValue("id")
	if id == "" {
		log.Printf(fmt.Sprintf("Инфо. На выход посутпил некорректный id(%v)", id))
		http.Error(w, "Полученненно не ID, а пустая строка", http.StatusBadRequest)
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

	buf := new(bytes.Buffer)

	var sizeOfContent int64 = 0

	w.Header().Add("Content-Disposition", "filename=\""+playList.Name+".zip"+"\"")
	zipWriter := zip.NewWriter(buf)

	for _, id := range playList.IDs {
		var song SongInfo
		err := songsColl.FindId(bson.ObjectIdHex(id)).One(&song)
		if err != nil {
			log.Println("Ошибка. При поиске записи в БД: " + err.Error())
			continue
		}

		fileWriter, err := zipWriter.Create(song.FileName)
		if err != nil {
			log.Println("Ошибка. При создании нового файла в архиве ошибка: " + err.Error())
			continue
		}

		data, err := ioutil.ReadFile(storageDirectory + id)
		if err != nil {
			log.Println("Ошибка. При чтении файла c id = " + id + " ошибка: " + err.Error())
			continue
		}
		n, err := fileWriter.Write(data)
		if err != nil {
			log.Println("Ошибка. При записи в файл архива: " + err.Error())
			continue
		}
		sizeOfContent += int64(n)
	}
	err = zipWriter.Close()
	if err != nil {
		log.Println("Ошибка. При закрытии архива: " + err.Error())
	}

	w.Header().Add("Content-Length", fmt.Sprintf("%v", sizeOfContent))
	w.Write(buf.Bytes())
	log.Println("Инфо. Закончилось успешно выполнение запроса на отдачу плэйлистов в zip")
}

// getMetadataOfSongsbyIDs - считывает переменную ids из запроса, в которой должен быть массив id,
// и возвращает массив метаданных об этих песнях.
func getMetadataOfSongsbyIDs(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу метаданные об массиве песен")

	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Content-type", "application/json;")

	idsString := r.FormValue("ids")
	if idsString == "" {
		log.Printf(fmt.Sprintf("Инфо. На выход посутпила пустая строка ids"))
		http.Error(w, "Полученненно не IDs, а пустая строка", http.StatusBadRequest)
		return
	}

	var ids []string
	json.Unmarshal([]byte(idsString), &ids)

	result := make([]SongInfo, 0, len(ids))
	for i := 0; i < len(ids); i++ {
		var metaData SongInfo
		if bson.IsObjectIdHex(ids[i]) {
			err := songsColl.FindId(bson.ObjectIdHex(ids[i])).One(&metaData)
			if err != nil {
				log.Println("Ошибка. При поиске записи в БД: " + err.Error())
				continue
			}
			result = append(result, metaData)
		}
	}

	data, err := json.Marshal(result)
	if err != nil {
		log.Println("Ошибка. При маршалинге в json результата: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
		return
	}

	_, err = w.Write(data)
	if err != nil {
		log.Println("Ошибка. При отдачи метоинформации: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	log.Println("Инфо. Закончилось успешно выполнение запроса на отдачу метаданные об массиве песен")
}

// getPlaylistInZip - создает zip архив, который содержит в себе указанные песни
func getSongsInZip(w http.ResponseWriter, r *http.Request) {
	log.Println("Инфо. Началось выполнение запроса на отдачу песен в zip")

	w.Header().Add("Access-Control-Allow-Origin", "*")
	//w.Header().Add("Content-type", "application/json;")

	jsonIDs := r.FormValue("ids")
	if jsonIDs == "" {
		log.Printf(fmt.Sprintf("Инфо. На выход посутпил пустой ids"))
		http.Error(w, "Полученненно не IDs, а пустая строка", http.StatusBadRequest)
		return
	}

	var ids []string
	err := json.Unmarshal([]byte(jsonIDs), &ids)
	if err != nil {
		log.Println("Ошибка. При анмаршалинге json: " + err.Error())
		http.Error(w, "Неполадки на сервере, повторите попытку позже", http.StatusInternalServerError)
	}

	buf := new(bytes.Buffer)

	var sizeOfContent int64 = 0
	w.Header().Add("Content-Disposition", "filename=\""+serviceName+time.Now().Format("15:04:05.000")+".zip"+"\"")
	zipWriter := zip.NewWriter(buf)

	for _, id := range ids {
		data, err := ioutil.ReadFile(storageDirectory + id)
		if err != nil {
			log.Println("Ошибка. При чтении файла c id = " + id + " ошибка: " + err.Error())
			continue
		}

		var song SongInfo
		err = songsColl.FindId(bson.ObjectIdHex(id)).One(&song)
		if err != nil {
			log.Println("Ошибка. При поиске записи в БД: " + err.Error())
			continue
		}

		fileWriter, err := zipWriter.Create(song.FileName)
		if err != nil {
			log.Println("Ошибка. При создании нового файла в архиве ошибка: " + err.Error())
			continue
		}

		n, err := fileWriter.Write(data)
		if err != nil {
			log.Println("Ошибка. При записи в файл архива: " + err.Error())
			continue
		}
		sizeOfContent += int64(n)
	}
	err = zipWriter.Close()
	if err != nil {
		log.Println("Ошибка. При закрытии архива: " + err.Error())
	}

	w.Header().Add("Content-Length", fmt.Sprintf("%v", sizeOfContent))
	w.Write(buf.Bytes())
	log.Println("Инфо. Закончилось успешно выполнение запроса на отдачу плэйлистов в zip")
}
