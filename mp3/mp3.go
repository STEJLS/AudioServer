package mp3

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"time"
)

type MP3meta struct {
	Title      string // название песни
	Artist     string // исполнитель
	Genre      string // жанр
	Bitrate    int    // килобит в секунду
	Duration   int    // продолжительность песни в секундах
	idv3v1tag  bool   // есть ли idv3v1tag(размер 128 байт с конца)
	idv3v2tag  bool   // есть ли idv3v2tag
	idv3v2size int    // размер idv3v1tag
}

//GetTitle - возвращает название песни(Возвращет пустую строку если название песни неизвестно)
func (mp3meta MP3meta) GetTitle() string {
	return mp3meta.Title
}

//GetArtist - возвращает имя исполнителя(Возвращет пустую строку если имя исполнителя неизвестно)
func (mp3meta MP3meta) GetArtist() string {
	return mp3meta.Artist
}

//GetGenre - возвращает название жанра(Возвращет пустую строку если название жанра неизвестно)
func (mp3meta MP3meta) GetGenre() string {
	return mp3meta.Genre
}

//GetBitrate - возвращает битрейт в билобайтах в секунду
func (mp3meta MP3meta) GetBitrate() int {
	return mp3meta.Bitrate
}

//GetDuration - возвращает продолжительность песни в секундах
func (mp3meta MP3meta) GetDuration() int {
	return mp3meta.Duration
}

func (t MP3meta) String() string {
	return fmt.Sprintf("title: '%v' \nartist: '%v' \ngenre:  '%v' \nBitrate:  '%v kbit/s' \nDuration:  '%v:%v'",
		t.Title, t.Artist, t.Genre, t.Bitrate, t.Duration/60, t.Duration%60)
}

//ParseMetadata парсит основную информацию о мп3 файле.
//Возвращает nil если это не mp3 файл.
func ParseMetadata(readSeeker io.ReadSeeker) *MP3meta {
	var file *MP3meta = new(MP3meta)

	//Работаем с ID3v1
	getID3v1Tags(readSeeker, file)

	//Работаем с ID3v2
	getID3v2Tags(readSeeker, file)

	//Работаем с фреймами mp3(цель вычислить длину песни и битрейт)
	err := getDurationAndBitRate(readSeeker, file)
	if err != nil {
		log.Println(err.Error())
		return nil
	}

	tryConvertToNewGenre(&file.Genre)

	return file
}

func getDurationAndBitRate(readSeeker io.ReadSeeker, file *MP3meta) error {

	//Ищем заголовок mp3 фрейма он идет после idv3v2tag, если он есть
	_, err := readSeeker.Seek(int64(file.idv3v2size), os.SEEK_SET)

	if err != nil {
		log.Println("При попытке переместиться на конец ID3V2 тэга: " + err.Error())
	}

	offset := searchOffsetForFirstMP3FrameHeader(readSeeker, 10000) //цифра придуманная...

	if offset == -1 {
		return errors.New("Заголовок MP3 не найден ")
	}

	_, err = readSeeker.Seek(int64(file.idv3v2size+offset), os.SEEK_SET)
	if err != nil {
		log.Println("При попытке переместиться на начало MP3 заголовка фрейма: " + err.Error())
	}
	//считываем заголовок первого фрейма---------------------------------     1
	data := make([]byte, 4)
	readSeeker.Read(data)

	var mp3header frameHeader
	err = mp3header.Parse(data)
	if err != nil {
		return errors.New("Заголовок MP3 не найден ")
	}

	//обнуляем счетчик времени
	duration := time.Duration(0)
	var bitRate uint64
	var count uint64
	//проверяем на VBR -------------------------------------------------      2
	data = make([]byte, mp3header.Size-4)
	readSeeker.Read(data)

	if !isVBR(data) {
		duration += mp3header.Duration
		bitRate += uint64(mp3header.Bitrate / 1000)
		count++
	}

	//считываем до конца ------------------------------------------------      3
	data = make([]byte, 4)
	readSeeker.Read(data)
	for mp3header.Parse(data) == nil {
		duration += mp3header.Duration
		_, err := readSeeker.Seek(mp3header.Size-4, os.SEEK_CUR)
		if err != nil {
			log.Println("При переходе на следующий фрейм MP3" + err.Error())
			break
		}
		_, err = readSeeker.Read(data)
		if err != nil {
			log.Println("При чтении следующего заголовка фрейма MP3" + err.Error())
			break
		}
		count++
		bitRate += uint64(mp3header.Bitrate / 1000)
	}

	file.Duration = round(duration.Seconds())
	file.Bitrate = int(bitRate / count)

	return nil
}

func searchOffsetForFirstMP3FrameHeader(readSeeker io.ReadSeeker, distance int) int {
	data := make([]byte, distance)

	n, err := readSeeker.Read(data)

	if n < 4 {
		log.Println("Кол-во считаных байт меньше чем размер заголовка.")
		return -1
	}
	if err != nil {
		log.Println(err.Error())
	}

	for i := 0; i < n-1; i++ {
		if data[i] == 0xFF && (data[i+1]&0xE0) == 0xE0 {
			return i
		}
	}
	return -1
}

// Попытка преобразовать жанр в формате "(NN)"
// в жанр в виде строки,
// где NN номер жанра первой версии.
func tryConvertToNewGenre(ganre *string) {
	if *ganre != "" {
		index := 0
		_, err := fmt.Sscanf(*ganre, "(%d)", &index)
		if err == nil {
			if index >= 0 && index < len(id3v1Genres) {
				*ganre = id3v1Genres[index]
			}
		}
	} else {
		*ganre = "Other"
	}

}

//VBR - variable bit rate
func isVBR(data []byte) bool {

	if len(data) < 4 {
		return false
	}

	idx := bytes.Index(data, []byte("Xing"))
	if idx != -1 {
		return true
	}
	idx = bytes.Index(data, []byte("Info"))
	if idx != -1 {
		return true
	}
	idx = bytes.Index(data, []byte("VBRI"))
	if idx != -1 {
		return true
	}

	return false
}

func round(f float64) int {
	if math.Abs(f) < 0.5 {
		return 0
	}
	return int(f + math.Copysign(0.5, f))
}
