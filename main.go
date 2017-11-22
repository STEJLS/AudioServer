package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/STEJLS/AudioServer/XMLconfig"
)

func main() {
	InitFlags()
	logFile := InitLogger(logSource)
	defer logFile.Close()

	config := XMLconfig.Get(configSource)

	connectToDB(config.Db.Host, config.Db.Port, config.Db.Name)
	defer audioDBsession.Close()

	server := http.Server{
		Addr: fmt.Sprintf(":%v", config.HTTP.Port),
	}

	http.HandleFunc("/addSong", addSong)
	http.HandleFunc("/addPlaylist", addPlaylist)
	http.HandleFunc("/getSong", getSong)
	http.HandleFunc("/getPlaylists", getPlaylists)
	http.HandleFunc("/getPlaylistInZip", getPlaylistInZip)
	http.HandleFunc("/getMetadataOfNewSongs", getMetadataOfNewSongs)
	http.HandleFunc("/getMetadataOfPopularSongs", getMetadataOfPopularSongs)
	http.HandleFunc("/searchSongs", searchSongs)
	http.HandleFunc("/searchPlaylists", searchPlaylists)
	http.HandleFunc("/addSongForm", addSongForm)
	http.HandleFunc("/getSongForm", getSongForm)
	http.HandleFunc("/addPlaylistForm", addPlaylistForm)
	http.HandleFunc("/getPopularSongsForm", getPopularSongsForm)
	http.HandleFunc("/searchPlaylistsForm", searchPlaylistsForm)

	err := server.ListenAndServe()
	if err != nil {
		log.Println(err.Error())
	}
}
