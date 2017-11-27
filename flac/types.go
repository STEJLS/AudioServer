package flac

import (
	"fmt"
	"io"
	"log"
)

type FlacMeta struct {
	Title    string // название песни
	Artist   string // исполнитель
	Genre    string // жанр
	Bitrate  int    // килобит в секунду
	Duration int    // продолжительность песни в секундах
}

func (flacMeta FlacMeta) String() string {
	return fmt.Sprintf("title: '%v' \nartist: '%v' \ngenre:  '%v' \nBitrate:  '%v kbit/s' \nDuration:  '%v:%v'",
		flacMeta.Title, flacMeta.Artist, flacMeta.Genre, flacMeta.Bitrate, flacMeta.Duration/60, flacMeta.Duration%60)
}

//GetTitle - возвращает название песни(Возвращет пустую строку если название песни неизвестно)
func (flacMeta FlacMeta) GetTitle() string {
	return flacMeta.Title
}

//GetArtist - возвращает имя исполнителя(Возвращет пустую строку если имя исполнителя неизвестно)
func (flacMeta FlacMeta) GetArtist() string {
	return flacMeta.Artist
}

//GetGenre - возвращает название жанра(Возвращет пустую строку если название жанра неизвестно)
func (flacMeta FlacMeta) GetGenre() string {
	return flacMeta.Genre
}

//GetBitrate - возвращает битрейт в билобайтах в секунду
func (flacMeta FlacMeta) GetBitrate() int {
	return flacMeta.Bitrate
}

//GetDuration - возвращает продолжительность песни в секундах
func (flacMeta FlacMeta) GetDuration() int {
	return flacMeta.Duration
}

type metaHeader struct {
	IsLast bool // Флаг последнего заголовка
	Type   int  // Тип метаданных
	Length int  // длина в байтах данных
}

//Parse - читает metadataHeaderSize байтов и парсит их в заголовок блока метаданных
func (meta *metaHeader) Parse(r io.Reader) error {

	buf := make([]byte, metadataHeaderSize)

	_, err := r.Read(buf)
	if err != nil {
		log.Println("Ошибка. При чтении заголовка метаданных: " + err.Error())
		return err
	}

	if buf[0]&(128)>>7 == 1 {
		meta.IsLast = true
	}

	meta.Type = int(buf[0] & (127))
	meta.Length = int(buf[1])<<16 | int(buf[2])<<8 | int(buf[3])

	return nil
}

// GetData - возвращает данные заголовка
// Если при чтении возникает ошибка, то возвращается nil
func (meta *metaHeader) GetData(r io.Reader) []byte {
	buf := make([]byte, meta.Length)

	_, err := r.Read(buf)
	if err != nil {
		log.Println("Ошибка. При чтении метаданных: " + err.Error())
		return nil
	}

	return buf
}

// streamInfo - содержит основные свойства потока аудио данных,
// лишние для нас данные убараны.
type streamInfo struct {
	SampleRate uint32 // Сэмплрейт в герцах
	NSamples   uint64 // Кол-во сэмплов во всем потоке
}

// Parse - читает streamInfoSize байтов и парсит их в streamInfo
func (info *streamInfo) Parse(r io.Reader) error {
	buf := make([]byte, streamInfoSize)

	_, err := r.Read(buf)
	if err != nil {
		log.Println("Ошибка. При чтении StreamInfo: " + err.Error())
		return err
	}

	info.SampleRate = (uint32(buf[10])<<16 | uint32(buf[11])<<8 | uint32(buf[12])) >> 4
	info.NSamples = uint64(buf[13]&15)<<32 | uint64(buf[14])<<24 | uint64(buf[15])<<16 | uint64(buf[16])<<8 | uint64(buf[17])

	return nil
}
