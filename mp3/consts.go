package mp3

//Константы касаемые id3 тэгов
const (
	frameV22HeaderSize    = 6
	frameV23V24HeaderSize = 10
	idv3v2HeaderSize      = 10
	id3v2HeaderPosition   = 0
	extendedHeaderSize    = 6
	id3v1Tagsize          = 128
)

//в Golang нельзя объъявить массив как константу....
var id3v1Genres = [...]string{
	"Blues", "Classic Rock", "Country", "Dance",
	"Disco", "Funk", "Grunge", "Hip-Hop",
	"Jazz", "Metal", "New Age", "Oldies",
	"Other", "Pop", "R&B", "Rap",
	"Reggae", "Rock", "Techno", "Industrial",
	"Alternative", "Ska", "Death Metal", "Pranks",
	"Soundtrack", "Euro-Techno", "Ambient", "Trip-Hop",
	"Vocal", "Jazz+Funk", "Fusion", "Trance",
	"Classical", "Instrumental", "Acid", "House",
	"Game", "Sound Clip", "Gospel", "Noise",
	"AlternRock", "Bass", "Soul", "Punk",
	"Space", "Meditative", "Instrumental Pop", "Instrumental Rock",
	"Ethnic", "Gothic", "Darkwave", "Techno-Industrial",
	"Electronic", "Pop-Folk", "Eurodance", "Dream",
	"Southern Rock", "Comedy", "Cult", "Gangsta",
	"Top 40", "Christian Rap", "Pop/Funk", "Jungle",
	"Native American", "Cabaret", "New Wave", "Psychadelic",
	"Rave", "Showtunes", "Trailer", "Lo-Fi",
	"Tribal", "Acid Punk", "Acid Jazz", "Polka",
	"Retro", "Musical", "Rock & Roll", "Hard Rock", "Folk", "Folk-Rock",
	"National Folk", "Swing", "Fast Fusion", "Bebob", "Latin", "Revival",
	"Celtic", "Bluegrass", "Avantgarde", "Gothic Rock", "Progressive Rock",
	"Psychedelic Rock", "Symphonic Rock", "Slow Rock", "Big Band", "Chorus",
	"Easy Listening", "Acoustic", "Humour", "Speech", "Chanson", "Opera",
	"Chamber Music", "Sonata", "Symphony", "Booty Bass", "Primus",
	"Porn Groove", "Satire", "Slow Jam", "Club", "Tango", "Samba", "Folklore",
	"Ballad", "Power Ballad", "Rhytmic Soul", "Freestyle", "Duet", "Punk Rock",
	"Drum Solo", "Acapella", "Euro-House", "Dance Hall", "Goa", "Drum & Bass",
	"Club-House", "Hardcore", "Terror", "Indie", "BritPop", "Negerpunk",
	"Polsk Punk", "Beat", "Christian Gangsta Rap", "Heavy Metal", "Black Metal",
	"Crossover", "Contemporary Christian", "Christian Rock",
	"Merengue", "Salsa", "Thrash Metal", "Anime", "Jpop", "Synthpop",
}

//Константы касаемые заголовка фрейма mp3
const (
	mPEG25 version = iota
	mPEGReserved
	mPEG2
	mPEG1
)

const (
	layerReserved layer = iota
	layer3
	layer2
	layer1
)

const (
	emphNone emphasis = iota
	emph5015
	emphReserved
	emphCCITJ17
)

const (
	stereo channelMode = iota
	jointStereo
	dualChannel
	singleChannel
)

var (
	bitrates = map[version]map[layer][15]int{
		mPEG1: { // MPEG 1
			layer1: {0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448}, // layer1
			layer2: {0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384},    // layer2
			layer3: {0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320},     // layer3
		},
		mPEG2: { // MPEG 2, 2.5
			layer1: {0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256}, // layer1
			layer2: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},      // layer2
			layer3: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},      // layer3
		},
	}
	sampleRates = map[version][3]int{
		mPEG1:        {44100, 48000, 32000},
		mPEG2:        {22050, 24000, 16000},
		mPEG25:       {11025, 12000, 8000},
		mPEGReserved: {0, 0, 0},
	}
	samplesPerFrame = map[version]map[layer]int{
		mPEG1: {
			layer1: 384,
			layer2: 1152,
			layer3: 1152,
		},
		mPEG2: {
			layer1: 384,
			layer2: 1152,
			layer3: 576,
		},
	}
	slotSize = map[layer]int{
		layerReserved: 0,
		layer3:        1,
		layer2:        1,
		layer1:        4,
	}
)
