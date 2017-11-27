package flac

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"math"
	"os"
	"strings"
)

// ParseMetadata - парсит метаданные flac
func ParseMetadata(rs io.ReadSeeker) *FlacMeta {
	meta := new(FlacMeta)

	err := findFlacMarker(rs)
	if err != nil {
		return nil
	}

	header := new(metaHeader)
	err = header.Parse(rs)
	if err != nil {
		return nil
	}

	info := new(streamInfo) //первым всегда идет блок streamInfo
	err = info.Parse(rs)
	if err != nil {
		return nil
	}

	meta.Duration = computeDuration(info)

	comment := getVorbisComment(rs)
	if comment != nil {
		parseVorbisComment(comment, meta)
	}

	meta.Bitrate = computeBitrate(meta, rs) // переводит seek на конец файла
	return meta
}

// findFlacMarker - осуществляет поиск маркера flac и устанавливает указатель на начало заголовка flac
func findFlacMarker(rs io.ReadSeeker) error {
	_, err := rs.Seek(0, os.SEEK_SET)
	if err != nil {
		log.Println("Ошибка. При переходе на начало файла для парсинга метаданных: " + err.Error())
		return err
	}

	buf := make([]byte, 4)
	_, err = rs.Read(buf)
	if err != nil {
		log.Println("Ошибка. При чтении предположительно маркера flac: " + err.Error())
		return err
	}

	if !bytes.Equal(buf, streamMarker) {
		err := advancedMarkerSearch(rs)
		if err != nil {
			log.Println("Ошибка. Это не flac")
			return err
		}
	}

	return nil
}

// advancedMarkerSearch - если в первых 4 батах файла нет маркера flac,
// то осуществляется его поиск в первых 100к байтах
func advancedMarkerSearch(rs io.ReadSeeker) error {
	data := make([]byte, advancedSearchLength)
	_, err := rs.Read(data)
	if err != nil {
		log.Println("Ошибка. При чтении данных для поиска маркера flac: " + err.Error())
		return err
	}

	for i := 0; i < advancedSearchLength-4; i++ {
		if bytes.Equal(data[i:i+4], streamMarker) {
			_, err := rs.Seek(int64(i+4), os.SEEK_SET)
			if err != nil {
				log.Println("Ошибка. При переходе на начало данных flac для парсинга метаданных: " + err.Error())
				return err
			}
			return nil
		}
	}

	return errors.New("Маркер flac не найден")
}

// computeDuration - считает длительность песни в секундах по формуле
// (1000/частота дискретизации)*кол-во сэмплов - время в миллисекундах
// делим на 1000 чтобы получить секунды
func computeDuration(info *streamInfo) int {
	return round(float64(info.NSamples) / float64(info.SampleRate))
}

// computeBitrate - вычисляет средний битрейт в kbps песни по формуле
// размер файла / длительность в секундах
func computeBitrate(meta *FlacMeta, rs io.ReadSeeker) int {
	n, err := rs.Seek(0, os.SEEK_END)
	if err != nil {
		log.Println("Ошибка. При переходе на конец файла для вычисления битрейта: " + err.Error())
		return 0
	}

	return int(n / 128 / int64(meta.Duration))
}

// getVorbisComment - читает заголовки метаданных и пропускает их
// до того момента пока не найдет Vorbis Comment.
// Если возникают какие-либо ошибки или такие метаданные не найдены, то возврщается nil
func getVorbisComment(rs io.ReadSeeker) []byte {
	header := new(metaHeader)
	for {
		err := header.Parse(rs)
		if err != nil {
			return nil
		}

		if header.Type == 4 { // VORBIS_COMMENT
			data := header.GetData(rs)
			if data == nil {
				return nil
			}

			return data
		}

		if header.IsLast {
			return nil
		}

		_, err = rs.Seek(int64(header.Length), os.SEEK_CUR)
		if err != nil {
			log.Println("Ошибка. При переходе на следующий заголовок метаданных" + err.Error())
			return nil
		}
	}
}

// parseVorbisComment - парсит ворбис коммент и извлекает Title, Artist, Ganre
func parseVorbisComment(comment []byte, meta *FlacMeta) {
	vendorLen := binary.LittleEndian.Uint32(comment[0:4])
	countOfComments := binary.LittleEndian.Uint32(comment[vendorLen+4 : vendorLen+8])
	pointer := int(vendorLen) + 8
	for i := 0; i < int(countOfComments); i++ {
		length := int(binary.LittleEndian.Uint32(comment[pointer : pointer+4]))
		pointer += 4
		pos := strings.Index(string(comment[pointer:pointer+length]), "=")

		if pos == -1 {
			continue
		}

		if string(comment[pointer:pointer+pos]) == "TITLE" {
			meta.Title = strings.TrimSpace(string(comment[pointer+pos+1 : pointer+length]))
		}

		if string(comment[pointer:pointer+pos]) == "ARTIST" {
			meta.Artist = strings.TrimSpace(string(comment[pointer+pos+1 : pointer+length]))
		}

		if string(comment[pointer:pointer+pos]) == "GENRE" {
			meta.Genre = strings.TrimSpace(string(comment[pointer+pos+1 : pointer+length]))
		}

		pointer += length
	}
}

func round(f float64) int {
	if math.Abs(f) < 0.5 {
		return 0
	}
	return int(f + math.Copysign(0.5, f))
}
