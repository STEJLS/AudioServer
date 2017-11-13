package XMLconfig

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
)

// Config - это основная структура для парсинга xml файла
type Config struct {
	HTTP Http     `xml:"http"`
	Db   DataBase `xml:"DataBase"`
}

// Http - это структура для парсинга
// информации об http из xml файла
type Http struct {
	XMLName xml.Name `xml:"http"`
	Port    int      `xml:"port,attr"`
	Host    string   `xml:"host,attr"`
}

// DataBase - это структура для парсинга
// информации об базеданных из xml файла
type DataBase struct {
	XMLName xml.Name `xml:"DataBase"`
	Host    string   `xml:"host"`
	Name    string   `xml:"name"`
	Port    int      `xml:"port"`
}

// Get - это функция парсит xml конфиг, находящийся в файле "source"
// а также проверяет его на правильность
func Get(source string) Config {
	data, err := ioutil.ReadFile(source)
	if err != nil {
		log.Fatalln(fmt.Sprintf("Фатал. При открытии xml файла(%v) для парсинга: ", source) + err.Error())
	}

	var config Config
	err = xml.Unmarshal(data, &config)
	if err != nil {
		log.Fatalln(fmt.Sprintf("Фатал. При анмаршалинге xml файла(%q): ", source) + err.Error())
	}

	log.Printf("Инфо. Файл %q успешно расперсен.", source)

	err = validating(config)
	if err != nil {
		log.Fatalln(err)
	}

	return config
}

// Validating - это функция которая проверяет введенную информацию из конфига
func validating(config Config) error {
	if config.HTTP.Port < 1024 || config.HTTP.Port >= 65535 {
		return fmt.Errorf("Фатал. Не валидный номер http порта(от 1024 до 65535), а вы ввели %v", config.HTTP.Port)
	}

	if strings.ContainsAny(config.Db.Name, "/\\.\"*<>:|?$,'") {
		return fmt.Errorf("Фатал. Не валидное имя базы данных(не должно быть символов /, \\, ., \", *, <, >, :, |, ?, $), введено: %q", config.Db.Name)
	}

	if config.Db.Port < 1024 || config.Db.Port >= 65535 {
		return fmt.Errorf("Фатал. Не валидный номер http порта(от 1024 до 65535), а вы ввели %v", config.HTTP.Port)
	}

	log.Printf("Инфо. Конфиг успешно прошел проверку.")
	return nil
}
