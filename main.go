package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/spf13/afero"
)

var (
	BackUpFrequency = time.Second * 1

	BackUpDir    = ".cloudbase"
	FileName     = "data"
	Data         = make(map[string][]interface{}, 0)
	paths        []string
	fs           = afero.NewOsFs()
	BackUpTicker *time.Ticker
)

func main() {
	exists, err := afero.Exists(fs, BackUpDir)
	if err != nil {
		log.Fatalln(err)
		return
	}
	if !exists {
		err = fs.MkdirAll(BackUpDir, 0740)
		if err != nil {
			log.Fatalln(err)
			return
		}
	} else {
		files, err := afero.ReadDir(fs, BackUpDir)
		if err != nil {
			fmt.Println("Cannot read dir:", err)
		}
		if err == nil && len(files) > 0 {
			var latest, which int
			for i, f := range files {

				nameArr := strings.Split(f.Name(), ".")
				if !strings.Contains(f.Name(), "cloudbase") {
					continue
				}
				time, err := strconv.Atoi(nameArr[0])
				if err != nil {
					log.Println("Cannot convert filename to time:", err)
					return
				}
				if time > latest {
					latest = time
					which = i
				}
				path := filepath.Join(BackUpDir, f.Name())
				paths = append(paths, path)
			}
			path := filepath.Join(BackUpDir, files[which].Name())
			file, err := fs.Open(path)
			if err != nil {
				log.Println("Cannot open file:", err)
				return
			}

			err = json.NewDecoder(file).Decode(&Data)
			if err != nil {
				log.Println("Cannot read file:", err)
				return
			}

			log.Println("Read backup file")
		}
	}
	router := httprouter.New()

	router.POST("/:key", HandlePush)
	router.PUT("/:key/:name/:value", HandleModify)
	router.GET("/:key", HandleGet)

	BackUpTicker = time.NewTicker(BackUpFrequency)
	go BackUpInterval()

	log.Println("Starting cloudbase on localhost:7777")
	log.Fatal(http.ListenAndServe(":7777", clean(router)))
}

func clean(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.Header)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", r.Header.Get("Access-Control-Request-Method"))
		handler.ServeHTTP(w, r)
	})
}

func BackUpInterval() {
	for t := range BackUpTicker.C {
		log.Println("Save at", t)
		BackUp()
		Clean()
	}
}

func JSONRead(r io.Reader) (interface{}, error) {
	var i interface{}
	d := json.NewDecoder(r)
	return i, d.Decode(&i)
}

func JSONWrite(w io.Writer, i interface{}) error {
	b, err := json.Marshal(i)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func HandlePush(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	key := ps.ByName("key")
	data, err := JSONRead(r.Body)
	if err != nil {
		log.Println("Cannot decode request:", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(http.StatusText(http.StatusBadRequest)))
		return
	}
	Push(key, data)
}

func HandleGet(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	JSONWrite(w, Get(ps.ByName("key")))
}

func HandleModify(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	key := ps.ByName("key")
	name := ps.ByName("name")
	value := ps.ByName("value")

	change, err := JSONRead(r.Body)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(http.StatusText(http.StatusBadRequest)))
		return
	}

	Modify(key, name, value, change)
}

func Push(key string, d interface{}) {
	Data[key] = append(Data[key], d)
	log.Println("Pushed", d, "to", key)
}

func Get(key string) interface{} {
	log.Println("Asked for", key)
	return Data[key]
}

func Modify(key, name string, value string, change interface{}) {
	for i, v := range Data[key] {
		if reflect.ValueOf(v).MapIndex(reflect.ValueOf(name)).Interface() == reflect.ValueOf(value).Interface() {
			log.Println("Modified", key, "with", name, value)
			Data[key][i] = change
		}
	}
}

func BackUp() {
	name := fmt.Sprintf("%d.cloudbase", time.Now().Unix())
	path := filepath.Join(BackUpDir, name)
	file, err := fs.Create(path)
	if err != nil {
		log.Println("Cannot save:", err)
		return
	}
	err = JSONWrite(file, Data)
	if err != nil {
		log.Println("Cannot save:", err)
		return
	}
	paths = append(paths, path)
}

func Clean() {
	if len(paths) > 60 {
		for _, p := range paths[1:] {
			err := fs.Remove(p)
			if err != nil {
				log.Println("Cannot clean", p, ":", err)
			}
			log.Println("Cleaned", p)
		}
		paths = []string{}
	}
}
