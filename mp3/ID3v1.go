//Package ID3v1 парсит последние 128 байт mp3 файла.
//http://id3.org/ID3v1 документация
package mp3

import (
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

//id3v1Tag - структура описывающая ID3v1 тэг(структура сразу для 2 версий тэга)
type id3v1Tag struct {
	title   string
	artist  string
	album   string
	year    int //0 - нет информации
	comment string
	track   byte //0 - нет информации
	genre   string
	version byte //0 - это ID3V1.1;  1 - это ID3V1.1
}

//string возвращает строковое представление тэга
func (t id3v1Tag) String() string {
	return fmt.Sprintf("title: %v \nartist: %v \nalbum: %v  \nyear: %v  \ncomment: %v \ntrack : %v \ngenre:  %v",
		t.title, t.artist, t.album, t.year, t.comment, t.track, t.genre)
}

//TrimSpace удаляет пробелы в концах строк тэга
func (t *id3v1Tag) trimSpace() {
	t.title = strings.Trim(t.title, string(32)+string(0))
	t.artist = strings.Trim(t.artist, string(32)+string(0))
	t.album = strings.Trim(t.album, string(32)+string(0))
	t.comment = strings.Trim(t.comment, string(32)+string(0))
}

//convertToUnicode Переводит строки тэга из Windows1251 в UTF-8
func (t *id3v1Tag) convertToUnicode() {
	decoder := charmap.Windows1251.NewDecoder()
	var err error
	t.title, err = decoder.String(t.title)
	if err != nil {
		log.Println(err.Error())
	}

	t.artist, err = decoder.String(t.artist)
	if err != nil {
		log.Println(err.Error())
	}

	t.album, err = decoder.String(t.album)
	if err != nil {
		log.Println(err.Error())
	}

	t.comment, err = decoder.String(t.comment)
	if err != nil {
		log.Println(err.Error())
	}
}

//Эта функция делает много лишнего, т.к. нам нужен только заголовок, жанр и исполнитель.
//Если дальнейшего расширеняи не будет, то можно выпилить лишний функционал.
func getID3v1Tags(readSeeker io.ReadSeeker, file *MP3meta) {
	file.idv3v1tag = false
	//Переходим на 128 байт с конца
	//и считываем ID3v1 тэг
	readSeeker.Seek(-id3v1Tagsize, os.SEEK_END)
	data := make([]byte, id3v1Tagsize)
	n, err := io.ReadFull(readSeeker, data)

	//Проверяем действительно ли это ID3v1
	if n != id3v1Tagsize {
		log.Printf("Считано %v, а размер заголовка idv3v1 %v\n", n, id3v1Tagsize)
		return
	}
	if err != nil {
		log.Println("Чтение ID3v1 тэга: " + err.Error())
		return
	}
	if string(data[:3]) != "TAG" {
		log.Println("Это не ID3v1: найдено '" + string(data[:3]) + "' , а должно быть TAG")
		return
	}

	//цифры кодируются сиволама из таблицы под номерами 48-57
	//если хоть один из 4 бит не
	var year int
	if data[96] > 48 && data[96] < 57 {
		year, err = strconv.Atoi(string(data[93:97]))
		if err != nil {
			log.Println(err.Error())
		}
	} else {
		year = 0
	}

	var ganre string
	//У нас есть 148 жанров(массив с 0 до 147)
	if data[127] < 147 {
		ganre = id3v1Genres[data[127]]
	} else {
		ganre = "Other"
	}

	t := id3v1Tag{
		title:  string(data[3:33]),
		artist: string(data[33:63]),
		album:  string(data[63:93]),
		year:   year,
		genre:  ganre,
	}

	if data[125] == 0 && data[126] != 0 {
		//29 байт комментария равен 0, а 30 не равен 0 - это ID3V1.1
		//30 байт - номер трека
		t.version = 1
		t.comment = string(data[97:125])
		t.track = data[126]
	} else {
		// Иначе это ID3V1.0 на комментарий, все 30 байт комментарий
		t.version = 1
		t.comment = string(data[97:127])
		t.track = 0
	}
	t.trimSpace()
	t.convertToUnicode()

	file.Title = t.title
	file.Artist = t.artist
	file.Genre = t.genre
	file.idv3v1tag = true
}
