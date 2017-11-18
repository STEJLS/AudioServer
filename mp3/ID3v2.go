package mp3

import (
	"io"
	"log"
	"os"
	"strings"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

type id3v2Header struct {
	Version           byte  // 4 байт
	SubVersion        byte  // 5 байт
	Unsynchronisation bool  // 6 байт 7 бит
	ExtendedHeader    bool  // 6 байт 6 бит
	Experimental      bool  // 6 байт 5 бит
	Footer            bool  // 6 байт 4 бит
	Size              int32 // 7-10 байты
}

type id3v2FrameHeader struct {
	name string
	size int32
}

// ID3v2/file identifier      "ID3"
// ID3v2 version              $04 00
// ID3v2 flags                %abcd0000
// ID3v2 size             4 * %0xxxxxxx

//Чтение тэгов ID3v2
//Возможна такая ситуация, когда не 1 тэг, а больше они идут друг за другом.
//Учитывается то что могут быть отступы после тэгов
//(тэг ищется из логики что отступ меньше чем сам тэг)
func getID3v2Tags(readSeeker io.ReadSeeker, file *MP3meta) {
	file.idv3v2size = 0
	file.idv3v2tag = false

	readSeeker.Seek(id3v2HeaderPosition, os.SEEK_SET)

	data := make([]byte, idv3v2HeaderSize)
	n, err := readSeeker.Read(data)

	if err != nil {
		log.Println(err.Error())
	}
	if n != idv3v2HeaderSize {
		log.Printf("Считано %v, а размер заголовка idv3v2 %v\n", n, idv3v2HeaderSize)
		return
	}

	for isID3V2header(data[:3]) {
		header := parseID3v2Header(data)

		if header.ExtendedHeader { //Если есть расширенный заголовок пропускаем его, он нам не нужен.
			size := getExtendedHeaderSize(readSeeker)
			readSeeker.Seek(int64(size+2), os.SEEK_CUR) //+2 - это 2 байта флаги
		}

		switch header.Version {
		case 2:
			getV22Tags(readSeeker, file)
			break
		case 3, 4:
			getV23_24Tags(readSeeker, file, header.Version)
			break
		default:
			if file.idv3v2size != 0 {
				file.idv3v2tag = true
			}
			return
		}

		file.idv3v2size += int(header.Size) + idv3v2HeaderSize

		_, err := readSeeker.Seek(int64(file.idv3v2size), os.SEEK_SET)
		if err != nil {
			log.Println("При попытке переместиться конец текущего ID3V2 заголовока: " + err.Error())
		}

		offset := searchOffsetForNextID3v2Header(readSeeker, header.Size)
		if offset == -1 { //заголовок не найден
			break
		}

		_, err = readSeeker.Seek(int64(file.idv3v2size+offset), os.SEEK_SET)
		if err != nil {
			log.Println("При попытке переместиться на следующий ID3V2 заголовок: " + err.Error())
		}

		readSeeker.Read(data)
	} //конец чтения тэгов
}

//-1 признак того что не найден заголовок
func searchOffsetForNextID3v2Header(readSeeker io.ReadSeeker, distance int32) int {
	data := make([]byte, distance)

	n, err := readSeeker.Read(data)
	if err != nil {
		log.Println("При поиске следующего заголовка ID3v2: " + err.Error())
		return -1
	}
	if n != int(distance) {
		log.Printf("Ищем второй и тд ID3v2 тэг. Считано %v, а размер заголовка idv3v2 %v\n", n, idv3v2HeaderSize)
		return -1
	}

	for i := 0; i < n-3; i++ {
		if string(data[i:i+3]) == "ID3" {
			return i
		}
	}
	return -1
}

// parseID3v2Header парсит заголовок ID3V2 тэга
func parseID3v2Header(data []byte) *id3v2Header {

	header := id3v2Header{
		Version:           data[3],
		SubVersion:        data[4],
		Unsynchronisation: data[5]&(128)>>7 == 1,
		ExtendedHeader:    data[5]&(64)>>6 == 1,
		Experimental:      data[5]&(32)>>5 == 1,
		Footer:            data[5]&(16)>>4 == 1,
		Size:              calculateTagSize(data[6:10]),
	}
	return &header
}

func isID3V2header(data []byte) bool {
	// идентификатор ID3 длиной 3 байта.
	if len(data) < 3 {
		return false
	}

	if string(data[:3]) != "ID3" {
		return false
	}

	return true
}

// старший байт всегда 0, поэтому он не учитывается
// A*2^21+B*2^14+C*2^7+D = A*2097152+B*16384+C*128+D,
// где  A первый байт, B второй,
// C 3 байт и D четвертый байт.
func calculateTagSize(data []byte) int32 {
	return int32(data[0])*2097152 + int32(data[1])*16384 + int32(data[2])*128 + int32(data[3])
}

//Перевод 4 байт в int32
func convertByteToInt(data []byte) int32 {
	return (int32(data[0])<<24 | int32(data[1])<<16 | int32(data[2])<<8 | int32(data[3]))
}

//Считаем размер расширенного заголовка
// Extended header size   4 * %0xxxxxxx
// Number of flag bytes       $01
// Extended Flags             $xx
func getExtendedHeaderSize(readSeeker io.ReadSeeker) int32 {
	data := make([]byte, extendedHeaderSize)
	readSeeker.Read(data)
	return calculateTagSize(data[:4])
}

// Читаем данные текстовых фреймов
// (Они начинаются с буквы "T")
func readTextFrame(destination *string, size int32, readSeeker io.ReadSeeker) {
	data := make([]byte, size)

	var err error
	_, err = readSeeker.Read(data)
	if err != nil {
		log.Println(err)
	}

	var s string
	switch data[0] {
	case 0: // ISO-8859-1 text.
		data, err = charmap.Windows1251.NewDecoder().Bytes(data[1:])
		// data, err = charmap.ISO8859_1.NewDecoder().Bytes(data[1:]) //написано в стандарте, но не кракозябры.
		break
	case 1: // UTF-16 with BOM.
		data, err = unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder().Bytes(data[1:])
		break
	case 2: // UTF-16BE without BOM.
		data, err = unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder().Bytes(data[1:])
	case 3: // UTF-8 text.
		data = data[1:]
		break
	default:
		// No encoding, assume ISO-8859-1 text.
		data, err = charmap.ISO8859_1.NewDecoder().Bytes(data[1:])
	}

	if err != nil {
		log.Println(err)
	}

	s = string(data)

	if s != "" {
		*destination = s
		*destination = strings.TrimRight(*destination, "\u0000"+string(0)+string(32))
	}
}
