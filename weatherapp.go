package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	_ "github.com/lib/pq"
)

const (
	host     = "localhost"
	port     = 5432
	user     = "postgres"
	password = "hireme123"
	dbname   = "postgres"
)

type Page struct {
	Title string
	Body  []byte
}

var Zip string

func (p *Page) save() error {
	filename := p.Title + ".txt"
	return ioutil.WriteFile(filename, p.Body, 0600)
}

func loadPage(title string) (*Page, error) {
	filename := title + ".txt"
	body, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return &Page{Title: title, Body: body}, nil
}

func viewHandler(w http.ResponseWriter, r *http.Request, title string) {
	data, err := query(Zip)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(data)

	reqBodyBytes := new(bytes.Buffer)
	json.NewEncoder(reqBodyBytes).Encode(data)
	filename := Zip + ".txt"
	ioutil.WriteFile(filename, reqBodyBytes.Bytes(), 0600)

}

func query(zip string) (weatherData, error) {
	resp, err := http.Get("http://api.openweathermap.org/data/2.5/weather?APPID=12ea4c57a813ac94155a0cabe7788b15&units=imperial&q=" + zip)
	if err != nil {
		return weatherData{}, err
	}

	defer resp.Body.Close()

	var d weatherData

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return weatherData{}, err
	}

	return d, nil

}

func cacheHandler(w http.ResponseWriter, r *http.Request, title string) {
	p, err := loadPage(title)
	if err != nil {
		http.Redirect(w, r, "/weatherapp/"+title, http.StatusFound)
		return
	}
	renderTemplate(w, "cache", p)
}

func weatherappHandler(w http.ResponseWriter, r *http.Request, title string) {
	p, err := loadPage(title)
	if err != nil {
		p = &Page{Title: title}
	}
	renderTemplate(w, "weatherapp", p)
}

func saveHandler(w http.ResponseWriter, r *http.Request, title string) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	address := r.PostFormValue("address")
	city := r.PostFormValue("city")
	state := r.PostFormValue("state")
	zip := r.PostFormValue("zip")
	Zip = zip

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		panic(err)
	}

	sqlStatement := `
	INSERT INTO addresses (Address, City, State, Zip)
	VALUES ($1, $2, $3, $4)`
	id := 0
	err = db.QueryRow(sqlStatement, address, city, state, zip).Scan(&id)

	filename := zip + ".txt"

	file, err := os.Stat(filename)

	if err != nil {
		fmt.Println(err)
	}
	if fileExists(filename) {
		modifiedtime := file.ModTime()

		now := time.Now()
		minus30 := time.Minute * time.Duration(-30)
		past := now.Add(minus30)

		if past.Before(modifiedtime) {
			http.Redirect(w, r, "/cache/"+Zip, http.StatusFound)
		} else {
			http.Redirect(w, r, "/view/"+Zip, http.StatusFound)
		}
	} else {
		http.Redirect(w, r, "/view/"+Zip, http.StatusFound)
	}

}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

var templates = template.Must(template.ParseFiles("weatherapp.html", "cache.html"))

func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
	err := templates.ExecuteTemplate(w, tmpl+".html", p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var validPath = regexp.MustCompile("^/(weatherapp|save|view|cache)/([a-zA-Z0-9]+)$")

func makeHandler(fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := validPath.FindStringSubmatch(r.URL.Path)
		if m == nil {
			http.NotFound(w, r)
			return
		}
		fn(w, r, m[2])
	}
}

func main() {
	http.HandleFunc("/view/", makeHandler(viewHandler))
	http.HandleFunc("/cache/", makeHandler(cacheHandler))
	http.HandleFunc("/weatherapp/", makeHandler(weatherappHandler))
	http.HandleFunc("/save/", makeHandler(saveHandler))

	log.Fatal(http.ListenAndServe(":8080", nil))
}

type weatherData struct {
	Name string `json:"name"`
	Main struct {
		Temperature float64 `json:"temp"`
		Mintemp     float64 `json:"temp_min"`
		Maxtemp     float64 `json:"temp_max"`
	} `json:"main"`
}
