package flac

const (
	metadataHeaderSize   int = 4      // Размер заголовка метаданных в байтах
	streamInfoSize       int = 34     // Размер блока STREAMINFO в байтах
	advancedSearchLength int = 100000 // Промежуток на котором ищется маркер flac
)

// Маркер потока FLAC
var streamMarker = []byte("fLaC")
