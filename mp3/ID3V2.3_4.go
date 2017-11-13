package mp3

import (
	"errors"
	"io"
	"os"
)

//Парсит ID3V2.3 и ID3V2.4 фреймы (четырехбуквенные идентификатора)
func getV23_24Tags(readSeeker io.ReadSeeker, file *MP3meta, version byte) {

	var header *id3v2FrameHeader = new(id3v2FrameHeader)
	for readFrameHeader(header, readSeeker, version) == nil {
		switch header.name {
		case "TIT2":
			readTextFrame(&file.Title, header.size, readSeeker)
			break
		case "TPE1":
			readTextFrame(&file.Artist, header.size, readSeeker)
			break
		case "TCON":
			readTextFrame(&file.Genre, header.size, readSeeker)
			break
		default:
			//Пропускаем ненужные фреймы
			readSeeker.Seek(int64(header.size), os.SEEK_CUR)
			break
		}
	}
}

// Frame ID   $xx xx xx xx  (четыре символа)
// Size       $xx xx xx xx
// Flags      $xx xx
// Если версия 3 то обычное четырехбайтное двоичное число
// Если версия 4 то как в заголовек ID3 (старший бит не считает он всегда 0)

func readFrameHeader(header *id3v2FrameHeader, readSeeker io.ReadSeeker, version byte) error {
	data := make([]byte, frameV23V24HeaderSize)

	_, err := readSeeker.Read(data)

	if err != nil {
		return err
	}

	for _, c := range data[:4] {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return errors.New("Это не фрейм")
		}
	}

	header.name = string(data[:4])

	switch version {
	case 3:
		header.size = convertByteToInt(data[4:8])
		break
	case 4:
		header.size = calculateTagSize(data[4:8])
		break
	default:
		header.size = convertByteToInt(data[4:8])
		break
	}
	//еще 2 байта флаги, они нам не нужны.
	return nil
}
