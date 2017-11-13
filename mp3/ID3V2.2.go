package mp3

import (
	"errors"
	"io"
	"os"
)

//Парсит ID3V2.2 фреймы (трехбуквенные идентификатора)
func getV22Tags(readSeeker io.ReadSeeker, file *MP3meta) {
	var header *id3v2FrameHeader = new(id3v2FrameHeader)
	for readv22FrameHeader(header, readSeeker) == nil {
		switch header.name {
		case "TT2":
			readTextFrame(&file.Title, header.size, readSeeker)
			break
		case "TP1":
			readTextFrame(&file.Artist, header.size, readSeeker)
			break
		case "TCO":
			readTextFrame(&file.Genre, header.size, readSeeker)
			break
		default:
			//Пропускаем ненужные фреймы
			readSeeker.Seek(int64(header.size), os.SEEK_CUR)
			break
		}
	}
}

// Frame ID   $xx xx xx  (три символа)
// Size       $xx xx xx (Хранится как обычное двоичное число)
//Длина заголовка фрейма 6 байт
//Длина идентификатора 3 байта
//Длина размера фрейма 3 байта
func readv22FrameHeader(header *id3v2FrameHeader, readSeeker io.ReadSeeker) error {
	data := make([]byte, frameV22HeaderSize)

	_, err := readSeeker.Read(data)
	if err != nil {
		return err
	}

	for _, c := range data[:3] {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return errors.New("Это не фрейм")
		}
	}

	header.name = string(data[:3])
	//Подгоняем под функцию convert3ByteToInt она
	//Она возвращает int32 а у нас только 24 байта
	//поэтому иммитируем 4-ый байт.
	data[2] = 0
	header.size = convertByteToInt(data[2:6])
	return nil
}
