package main

import "net/http"

func addSongForm(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`<html>
		<head>
		<meta http-equiv="Content-Type" content="text/html; charset=utf-8" />
		<title>AudioServer</title>
		</head>
		<body>
		<form action="/addSong" 
		method="post" enctype="multipart/form-data">
		<input type="file" name="file">
		<input type="submit"/>
		</form>
		</body>
		</html>`),
	)
}

func getSongForm(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`<html>
		<head>
		<meta http-equiv="Content-Type" content="text/html; charset=utf-8" />
		<title>AudioServer</title>
		</head>
		<body>
		<form action="/getSong" 
		method="post" enctype="application/x-www-form-urlencoded">
		<input type="text" name="id">
		<input type="submit"/>
		</form>
		</body>
		</html>`),
	)
}

func getPopularSongsForm(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`<html>
		<head>
		<meta http-equiv="Content-Type" content="text/html; charset=utf-8" />
		<title>AudioServer</title>
		</head>
		<body>
		<form action="/getMetadataOfPopularSongs" 
		method="post" enctype="application/x-www-form-urlencoded">
		<input type="text" name="count">
		<input type="submit"/>
		</form>
		</body>
		</html>`),
	)
}
